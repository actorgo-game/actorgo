// Package file from https://github.com/gogf/gf
package cmap

import (
	"encoding/json"
	"maps"
	"slices"
	"sync"
	"unsafe"

	cutils "github.com/actorgo-game/actorgo/extend/utils"
)

type StringAnyMap struct {
	mu   *sync.RWMutex
	data map[string]any
}

// NewStrAnyMap returns an empty StrAnyMap object.
// The parameter <safe> is used to specify whether using map in concurrent-safety,
// which is false in default.
func NewStrAnyMap() *StringAnyMap {
	return &StringAnyMap{
		mu:   &sync.RWMutex{},
		data: make(map[string]any),
	}
}

// NewStrAnyMapFrom creates and returns a hash map from given map <data>.
// Note that, the param <data> map will be set as the underlying data map(no deep copy),
// there might be some concurrent-safe issues when changing the map outside.
func NewStrAnyMapFrom(data map[string]any) *StringAnyMap {
	return &StringAnyMap{
		mu:   &sync.RWMutex{},
		data: data,
	}
}

// Iterator iterates the hash map readonly with custom callback function <f>.
// If <f> returns true, then it continues iterating; or false to stop.
func (m *StringAnyMap) Iterator(f func(k string, v any) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.data {
		if !f(k, v) {
			break
		}
	}
}

// Clone returns a new hash map with copy of current map data.
func (m *StringAnyMap) Clone() *StringAnyMap {
	return NewStrAnyMapFrom(m.MapCopy())
}

// Map returns a copy of the underlying data map.
func (m *StringAnyMap) Map() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return maps.Clone(m.data)
}

// MapStrAny returns a copy of the underlying data of the map as map[string]any.
func (m *StringAnyMap) MapStrAny() map[string]any {
	return m.Map()
}

// MapCopy returns a copy of the underlying data of the hash map.
func (m *StringAnyMap) MapCopy() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return maps.Clone(m.data)
}

// FilterEmpty deletes all key-value pair of which the value is empty.
// Values like: 0, nil, false, "", len(slice/map/chan) == 0 are considered empty.
func (m *StringAnyMap) FilterEmpty() {
	m.mu.Lock()
	for k, v := range m.data {
		if cutils.IsEmpty(v) {
			delete(m.data, k)
		}
	}
	m.mu.Unlock()
}

// FilterNil deletes all key-value pair of which the value is nil.
func (m *StringAnyMap) FilterNil() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.data {
		if cutils.IsNil(v) {
			delete(m.data, k)
		}
	}
}

// Set sets key-value to the hash map.
func (m *StringAnyMap) Set(key string, val any) {
	m.mu.Lock()
	if m.data == nil {
		m.data = make(map[string]any)
	}
	m.data[key] = val
	m.mu.Unlock()
}

// Sets batch sets key-values to the hash map.
func (m *StringAnyMap) Sets(data map[string]any) {
	m.mu.Lock()
	if m.data == nil {
		m.data = data
	} else {
		maps.Copy(m.data, data)
	}
	m.mu.Unlock()
}

// Search searches the map with given <key>.
// Second return parameter <found> is true if key was found, otherwise false.
func (m *StringAnyMap) Search(key string) (value any, found bool) {
	m.mu.RLock()
	if m.data != nil {
		value, found = m.data[key]
	}
	m.mu.RUnlock()
	return
}

// Get returns the value by given <key>.
func (m *StringAnyMap) Get(key string) (value any) {
	m.mu.RLock()
	if m.data != nil {
		value = m.data[key]
	}
	m.mu.RUnlock()
	return
}

// Pop retrieves and deletes an item from the map.
func (m *StringAnyMap) Pop() (key string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value = range m.data {
		delete(m.data, key)
		return
	}
	return
}

// Pops retrieves and deletes <size> items from the map.
// It returns all items if size == -1.
func (m *StringAnyMap) Pops(size int) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if size > len(m.data) || size == -1 {
		size = len(m.data)
	}
	if size == 0 {
		return nil
	}
	var (
		index  = 0
		newMap = make(map[string]any, size)
	)
	for k, v := range m.data {
		delete(m.data, k)
		newMap[k] = v
		index++
		if index == size {
			break
		}
	}
	return newMap
}

// doSetWithLockCheck checks whether value of the key exists with mutex.Lock,
// if not exists, set value to the map with given <key>,
// or else just return the existing value.
//
// When setting value, if <value> is type of <func() interface {}>,
// it will be executed with mutex.Lock of the hash map,
// and its return value will be set to the map with <key>.
//
// It returns value with given <key>.
func (m *StringAnyMap) doSetWithLockCheck(key string, value any) any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]any)
	}
	if v, ok := m.data[key]; ok {
		return v
	}
	if f, ok := value.(func() any); ok {
		value = f()
	}
	if value != nil {
		m.data[key] = value
	}
	return value
}

// GetOrSet returns the value by key,
// or sets value with given <value> if it does not exist and then returns this value.
func (m *StringAnyMap) GetOrSet(key string, value any) any {
	if v, ok := m.Search(key); !ok {
		return m.doSetWithLockCheck(key, value)
	} else {
		return v
	}
}

// GetOrSetFunc returns the value by key,
// or sets value with returned value of callback function <f> if it does not exist
// and then returns this value.
func (m *StringAnyMap) GetOrSetFunc(key string, f func() any) any {
	if v, ok := m.Search(key); !ok {
		return m.doSetWithLockCheck(key, f())
	} else {
		return v
	}
}

// GetOrSetFuncLock returns the value by key,
// or sets value with returned value of callback function <f> if it does not exist
// and then returns this value.
//
// GetOrSetFuncLock differs with GetOrSetFunc function is that it executes function <f>
// with mutex.Lock of the hash map.
func (m *StringAnyMap) GetOrSetFuncLock(key string, f func() any) any {
	if v, ok := m.Search(key); !ok {
		return m.doSetWithLockCheck(key, f)
	} else {
		return v
	}
}

// GetVar returns a Var with the value by given <key>.
// The returned Var is un-concurrent safe.
func (m *StringAnyMap) GetVar(key string) any {
	return m.Get(key)
}

// GetVarOrSet returns a Var with result from GetVarOrSet.
// The returned Var is un-concurrent safe.
func (m *StringAnyMap) GetVarOrSet(key string, value any) any {
	return m.GetOrSet(key, value)
}

// GetVarOrSetFunc returns a Var with result from GetOrSetFunc.
// The returned Var is un-concurrent safe.
func (m *StringAnyMap) GetVarOrSetFunc(key string, f func() any) any {
	return m.GetOrSetFunc(key, f)
}

// GetVarOrSetFuncLock returns a Var with result from GetOrSetFuncLock.
// The returned Var is un-concurrent safe.
func (m *StringAnyMap) GetVarOrSetFuncLock(key string, f func() any) any {
	return m.GetOrSetFuncLock(key, f)
}

// SetIfNotExist sets <value> to the map if the <key> does not exist, and then returns true.
// It returns false if <key> exists, and <value> would be ignored.
func (m *StringAnyMap) SetIfNotExist(key string, value any) bool {
	if !m.Contains(key) {
		m.doSetWithLockCheck(key, value)
		return true
	}
	return false
}

// SetIfNotExistFunc sets value with return value of callback function <f>, and then returns true.
// It returns false if <key> exists, and <value> would be ignored.
func (m *StringAnyMap) SetIfNotExistFunc(key string, f func() any) bool {
	if !m.Contains(key) {
		m.doSetWithLockCheck(key, f())
		return true
	}
	return false
}

// SetIfNotExistFuncLock sets value with return value of callback function <f>, and then returns true.
// It returns false if <key> exists, and <value> would be ignored.
//
// SetIfNotExistFuncLock differs with SetIfNotExistFunc function is that
// it executes function <f> with mutex.Lock of the hash map.
func (m *StringAnyMap) SetIfNotExistFuncLock(key string, f func() any) bool {
	if !m.Contains(key) {
		m.doSetWithLockCheck(key, f)
		return true
	}
	return false
}

// Removes batch deletes values of the map by keys.
func (m *StringAnyMap) Removes(keys []string) {
	m.mu.Lock()
	if m.data != nil {
		for _, key := range keys {
			delete(m.data, key)
		}
	}
	m.mu.Unlock()
}

// Remove deletes value from map by given <key>, and return this deleted value.
func (m *StringAnyMap) Remove(key string) (value any) {
	m.mu.Lock()
	if m.data != nil {
		var ok bool
		if value, ok = m.data[key]; ok {
			delete(m.data, key)
		}
	}
	m.mu.Unlock()
	return
}

// Keys returns all keys of the map as a slice.
func (m *StringAnyMap) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Collect(maps.Keys(m.data))
}

// Values returns all values of the map as a slice.
func (m *StringAnyMap) Values() []any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Collect(maps.Values(m.data))
}

// Contains checks whether a key exists.
// It returns true if the <key> exists, or else false.
func (m *StringAnyMap) Contains(key string) bool {
	var ok bool
	m.mu.RLock()
	if m.data != nil {
		_, ok = m.data[key]
	}
	m.mu.RUnlock()
	return ok
}

// Size returns the size of the map.
func (m *StringAnyMap) Size() int {
	m.mu.RLock()
	length := len(m.data)
	m.mu.RUnlock()
	return length
}

// IsEmpty checks whether the map is empty.
// It returns true if map is empty, or else false.
func (m *StringAnyMap) IsEmpty() bool {
	return m.Size() == 0
}

// Clear deletes all data of the map.
func (m *StringAnyMap) Clear() {
	m.mu.Lock()
	clear(m.data)
	m.mu.Unlock()
}

// Replace the data of the map with given <data>.
func (m *StringAnyMap) Replace(data map[string]any) {
	m.mu.Lock()
	m.data = data
	m.mu.Unlock()
}

// LockFunc locks writing with given callback function <f> within RWMutex.Lock.
func (m *StringAnyMap) LockFunc(f func(m map[string]any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f(m.data)
}

// RLockFunc locks reading with given callback function <f> within RWMutex.RLock.
func (m *StringAnyMap) RLockFunc(f func(m map[string]any)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f(m.data)
}

// Merge merges two hash maps.
// The <other> map will be merged into the map <m>.
func (m *StringAnyMap) Merge(other *StringAnyMap) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = other.MapCopy()
		return
	}
	if other != m {
		other.mu.RLock()
		defer other.mu.RUnlock()
	}
	maps.Copy(m.data, other.data)
}

// String returns the map as a string.
func (m *StringAnyMap) String() string {
	b, _ := m.MarshalJSON()
	return *(*string)(unsafe.Pointer(&b))
}

// MarshalJSON implements the interface MarshalJSON for json.Marshal.
func (m *StringAnyMap) MarshalJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.data)
}

// UnmarshalJSON implements the interface UnmarshalJSON for json.Unmarshal.
func (m *StringAnyMap) UnmarshalJSON(b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]any)
	}
	if err := json.Unmarshal(b, &m.data); err != nil {
		return err
	}
	return nil
}
