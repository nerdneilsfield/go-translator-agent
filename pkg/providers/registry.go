package providers

import (
	"fmt"
	"sync"
)

// Registry 提供商注册表
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry 创建新的注册表
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register 注册提供商
func (r *Registry) Register(name string, provider Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %s already registered", name)
	}

	r.providers[name] = provider
	return nil
}

// Get 获取提供商
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", name)
	}

	return provider, nil
}

// List 列出所有提供商
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}

	return names
}

// Remove 移除提供商
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.providers, name)
}

// Clear 清空注册表
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers = make(map[string]Provider)
}

// DefaultRegistry 默认注册表
var DefaultRegistry = NewRegistry()

// Register 注册到默认注册表
func Register(name string, provider Provider) error {
	return DefaultRegistry.Register(name, provider)
}

// Get 从默认注册表获取
func Get(name string) (Provider, error) {
	return DefaultRegistry.Get(name)
}

// List 列出默认注册表中的提供商
func List() []string {
	return DefaultRegistry.List()
}
