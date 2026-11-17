// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/core/server"
)

// AutoHTTPServer is an Extension that reads all publish.PublishedService entries
// with publish.HTTPServerCap and builds an HTTP server automatically.
//
// For each service, each publishable method gets a POST /{method-name} handler.
// Health endpoint (GET /health) is added if the capability requests it.
//
// Host/port are resolved from the service's config struct by looking for
// ServerHost/ServerPort fields (convention). Falls back to 127.0.0.1:8080.
func AutoHTTPServer(appName, version string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("auto.http", "", func(app *application.Application) error {
			f := &autoHTTPFactory{app: app, appName: appName, version: version}
			application.PutService[server.LifecycleFactory](app, "default", f.Create)
			return nil
		})
	}
}

type autoHTTPFactory struct {
	app     *application.Application
	appName string
	version string
}

func (f *autoHTTPFactory) Create() (*server.Lifecycle, error) {
	entries := application.UseAppExtensions[publish.PublishedService](f.app)
	if len(entries) == 0 {
		return nil, fmt.Errorf("auto-http: no published services point")
	}

	var addr string
	var host string
	var port int
	var bindings []httpServiceBinding

	for _, ps := range entries {
		cap := ps.FindCapability("http-server")
		if cap == nil {
			continue
		}

		impl, err := resolveServiceByType(f.app, ps.ServiceType)
		if err != nil {
			return nil, fmt.Errorf("auto-http: resolve %s: %w", ps.Name, err)
		}

		httpCap := cap.(publish.HTTPServerCap)

		// Try to resolve addr from config file (convention: server_host/server_port fields)
		if addr == "" {
			host, port = resolveServerAddr(f.app, ps.Name)
			// Apply capability defaults if config didn't provide values
			if host == "" {
				host = httpCap.DefaultHost
			}
			if port == 0 {
				port = httpCap.DefaultPort
			}
			if host != "" && port != 0 {
				addr = fmt.Sprintf("%s:%d", host, port)
			}
		}

		methods := DiscoverMethods(ps.ServiceType)
		bindings = append(bindings, httpServiceBinding{
			name:    ps.Name,
			impl:    impl,
			methods: methods,
			cap:     httpCap,
		})
	}

	if len(bindings) == 0 {
		return nil, fmt.Errorf("auto-http: no services with http-server capability")
	}

	if addr == "" {
		host = "127.0.0.1"
		port = 8080
		addr = "127.0.0.1:8080"
	}

	return &server.Lifecycle{
		Name:       f.appName,
		SubCommand: "server",
		Host:       host,
		Port:       port,
		URL:        fmt.Sprintf("http://%s", addr),
		Version:    f.version,
		FgArgs:     []string{"server", "start", "--fg"},
		Start:      buildAutoHTTPStartFunc(addr, bindings, f.appName, f.version),
	}, nil
}

// resolveServerAddr resolves host/port from the service instance.
// Checks if the service config has an Addr() method (convention), then parses it.
// Falls back to UseConfig to get the raw config with defaults applied.
func resolveServerAddr(app *application.Application, serviceName string) (string, int) {
	// Try standalone config by service name
	cfgMap, ok := application.GetConfigMap(app, serviceName)
	if !ok {
		return "", 0
	}
	host, _ := cfgMap["server_host"].(string)
	port := toInt(cfgMap["server_port"])
	if host == "" {
		host = "127.0.0.1"
	}
	return host, port
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	default:
		return 0
	}
}

type httpServiceBinding struct {
	name    string
	impl    any
	methods []ServiceMethod
	cap     publish.HTTPServerCap
}

func buildAutoHTTPStartFunc(addr string, bindings []httpServiceBinding, appName, version string) server.StartFunc {
	return func(ctx context.Context) error {
		mux := http.NewServeMux()

		for _, binding := range bindings {
			implVal := reflect.ValueOf(binding.impl)
			for _, sm := range binding.methods {
				pattern := fmt.Sprintf("POST /%s", corereflect.ToKebab(sm.Name))
				handler := buildHTTPHandler(implVal, sm)
				mux.HandleFunc(pattern, handler)
			}

			if binding.cap.HealthEndpoint {
				mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
					writeJSON(w, http.StatusOK, map[string]string{
						"status":  "ok",
						"service": appName,
						"version": version,
					})
				})
			}
		}

		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{
				"service": appName,
				"version": version,
			})
		})

		srv := &http.Server{Addr: addr, Handler: mux}

		go func() {
			<-ctx.Done()
			srv.Shutdown(context.Background())
		}()

		return srv.ListenAndServe()
	}
}

func buildHTTPHandler(implVal reflect.Value, sm ServiceMethod) http.HandlerFunc {
	method := implVal.MethodByName(sm.Name)

	return func(w http.ResponseWriter, r *http.Request) {
		var results []reflect.Value

		if sm.InputType == nil {
			// No-input method
			results = method.Call([]reflect.Value{reflect.ValueOf(r.Context())})
		} else {
			// With-input method
			inPtr := reflect.New(sm.InputType)
			if r.Body != nil {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				if len(data) > 0 {
					if err := json.Unmarshal(data, inPtr.Interface()); err != nil {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
						return
					}
				}
			}
			results = method.Call([]reflect.Value{reflect.ValueOf(r.Context()), inPtr.Elem()})
		}

		if !results[1].IsNil() {
			err := results[1].Interface().(error)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, results[0].Interface())
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
