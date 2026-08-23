package state

import (
	"container/list"
	"time"
)

type cache[K comparable, V any] struct {
	maxEntries int
	items      map[K]*cacheItem[K, V]
	order      *list.List
}

type cacheItem[K comparable, V any] struct {
	key       K
	value     V
	expiresAt time.Time
	element   *list.Element
}

func newCache[K comparable, V any](maxEntries int) *cache[K, V] {
	if maxEntries <= 0 {
		maxEntries = 1
	}
	return &cache[K, V]{
		maxEntries: maxEntries,
		items:      map[K]*cacheItem[K, V]{},
		order:      list.New(),
	}
}

func (c *cache[K, V]) put(key K, value V, expiresAt time.Time) (K, bool) {
	var zero K
	if item, ok := c.items[key]; ok {
		item.value = value
		item.expiresAt = expiresAt
		c.order.MoveToFront(item.element)
		return zero, false
	}
	item := &cacheItem[K, V]{
		key:       key,
		value:     value,
		expiresAt: expiresAt,
	}
	item.element = c.order.PushFront(item)
	c.items[key] = item
	for len(c.items) > c.maxEntries {
		back := c.order.Back()
		if back == nil {
			return zero, false
		}
		evicted := back.Value.(*cacheItem[K, V])
		delete(c.items, evicted.key)
		c.order.Remove(back)
		return evicted.key, true
	}
	return zero, false
}

func (c *cache[K, V]) get(key K, now time.Time) (V, bool) {
	var zero V
	item, ok := c.items[key]
	if !ok {
		return zero, false
	}
	if !item.expiresAt.IsZero() && !now.Before(item.expiresAt) {
		c.delete(key)
		return zero, false
	}
	c.order.MoveToFront(item.element)
	return item.value, true
}

func (c *cache[K, V]) delete(key K) (V, bool) {
	var zero V
	item, ok := c.items[key]
	if !ok {
		return zero, false
	}
	delete(c.items, key)
	c.order.Remove(item.element)
	return item.value, true
}

func (c *cache[K, V]) purgeExpired(now time.Time) []K {
	keys := []K{}
	for key, item := range c.items {
		if item.expiresAt.IsZero() || now.Before(item.expiresAt) {
			continue
		}
		keys = append(keys, key)
		c.order.Remove(item.element)
		delete(c.items, key)
	}
	return keys
}

func (c *cache[K, V]) values(now time.Time) []V {
	values := make([]V, 0, len(c.items))
	for key, item := range c.items {
		if !item.expiresAt.IsZero() && !now.Before(item.expiresAt) {
			c.delete(key)
			continue
		}
		values = append(values, item.value)
	}
	return values
}

func (c *cache[K, V]) len() int {
	return len(c.items)
}
