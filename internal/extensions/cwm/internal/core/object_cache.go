// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// objectFiles keeps each object file's last reading with its size and time:
// a file not changed since is answered from memory, never read and parsed
// again (- a planning view reads the same objects on every
// opening). A write lands through a rename, which gives the file a new time,
// and a file written too recently to tell is never kept, so a changed file is
// always read again; each answer is a deep copy, so no
// caller changes what another reads.
var objectFiles = struct {
	sync.Mutex
	m map[string]cachedObject
}{m: map[string]cachedObject{}}

type cachedObject struct {
	mod  time.Time
	size int64
	obj  api.Object
	rev  string
}

// cachedObjectFile is the object a file held when last read, if the file has
// not changed since.
func cachedObjectFile(path string) (os.FileInfo, api.Object, string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, api.Object{}, "", false
	}
	objectFiles.Lock()
	c, ok := objectFiles.m[path]
	objectFiles.Unlock()
	if !ok || c.size != info.Size() || !c.mod.Equal(info.ModTime()) {
		return info, api.Object{}, "", false
	}
	return info, deepCopy(c.obj), c.rev, true
}

// racyWindow is how recent a file's time may be for its reading to be kept:
// an edit within the clock's tick of the previous write (or a coarse clock's
// second) can keep both the size and the time, so a file written that
// recently is read again every time (: no cache serves a value older
// than the last change). Only a tool that sets the time back on purpose to an
// older one, with the same size, goes unseen.
const racyWindow = 3 * time.Second

// keepObjectFile remembers what a file held, by the size and time it had
// before it was read: a file changed meanwhile has another time, so the next
// reading reads it again. A file whose time falls within racyWindow of now
// is not kept: a later edit could not be told apart by its time.
func keepObjectFile(path string, info os.FileInfo, obj api.Object, rev string) {
	if info == nil || time.Since(info.ModTime()) < racyWindow {
		return
	}
	objectFiles.Lock()
	objectFiles.m[path] = cachedObject{mod: info.ModTime(), size: info.Size(), obj: deepCopy(obj), rev: rev}
	objectFiles.Unlock()
}

// deepCopy copies a value with every map, slice and pointer in it.
func deepCopy[T any](v T) T {
	src := reflect.ValueOf(&v).Elem()
	dst := reflect.New(src.Type()).Elem()
	copyValue(dst, src)
	return dst.Interface().(T)
}

func copyValue(dst, src reflect.Value) {
	switch src.Kind() {
	case reflect.Map:
		if src.IsNil() {
			return
		}
		m := reflect.MakeMapWithSize(src.Type(), src.Len())
		it := src.MapRange()
		for it.Next() {
			v := reflect.New(src.Type().Elem()).Elem()
			copyValue(v, it.Value())
			m.SetMapIndex(it.Key(), v)
		}
		dst.Set(m)
	case reflect.Slice:
		if src.IsNil() {
			return
		}
		s := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			copyValue(s.Index(i), src.Index(i))
		}
		dst.Set(s)
	case reflect.Pointer:
		if src.IsNil() {
			return
		}
		p := reflect.New(src.Type().Elem())
		copyValue(p.Elem(), src.Elem())
		dst.Set(p)
	case reflect.Interface:
		if src.IsNil() {
			return
		}
		v := reflect.New(src.Elem().Type()).Elem()
		copyValue(v, src.Elem())
		dst.Set(v)
	case reflect.Struct:
		t := src.Type()
		for i := 0; i < t.NumField(); i++ {
			if !t.Field(i).IsExported() {
				dst.Set(src) // a struct with unexported fields (a time) is a value
				return
			}
		}
		for i := 0; i < t.NumField(); i++ {
			copyValue(dst.Field(i), src.Field(i))
		}
	default:
		dst.Set(src)
	}
}
