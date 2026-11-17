// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// homeDir is the application home directory, set via Init.
var homeDir string

// Init sets the home directory used for servers/ and logs/ subdirectories.
// Must be called before any other server package function.
func Init(home string) {
	homeDir = home
}

// ServerInfo holds metadata about a running server instance.
// Written to a JSON file in the servers directory on start, removed on stop.
type ServerInfo struct {
	Name    string    `json:"name"`
	PID     int       `json:"pid"`
	URL     string    `json:"url"`
	Host    string    `json:"host"`
	Port    int       `json:"port"`
	Started time.Time `json:"started"`
	Version string    `json:"version"`
	Exe     string    `json:"exe"`
}

// ServersDir returns the path to the servers directory.
func ServersDir() string {
	return filepath.Join(homeDir, "servers")
}

// InfoPath returns the path to the info file for the named server.
func InfoPath(name string) string {
	return filepath.Join(ServersDir(), name+".json")
}

// WriteInfo writes the server info file atomically.
func WriteInfo(info ServerInfo) error {
	dir := ServersDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create servers dir: %w", err)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal server info: %w", err)
	}

	path := InfoPath(info.Name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write server info: %w", err)
	}
	return nil
}

// ReadInfo reads the server info file for the named server.
// Returns os.ErrNotExist if no info file exists.
func ReadInfo(name string) (ServerInfo, error) {
	data, err := os.ReadFile(InfoPath(name))
	if err != nil {
		return ServerInfo{}, err
	}

	var info ServerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ServerInfo{}, fmt.Errorf("parse server info: %w", err)
	}
	return info, nil
}

// RemoveInfo deletes the server info file.
func RemoveInfo(name string) error {
	return os.Remove(InfoPath(name))
}

// ListServers returns info for all server info files in the servers directory.
func ListServers() ([]ServerInfo, error) {
	dir := ServersDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var servers []ServerInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := e.Name()[:len(e.Name())-5] // strip .json
		info, err := ReadInfo(name)
		if err != nil {
			continue
		}
		servers = append(servers, info)
	}
	return servers, nil
}
