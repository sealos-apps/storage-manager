package state

import (
	"container/list"
	"crypto/subtle"
	"fmt"
	"sync"
	"time"

	"github.com/nixieboluo/sealos-storage-manager/internal/config"
	"github.com/nixieboluo/sealos-storage-manager/internal/domain"
)

type Store struct {
	mu sync.Mutex

	podSessions     *cache[string, *domain.PodSession]
	viewerSessions  *cache[string, *domain.ViewerSession]
	authRequests    *cache[string, *domain.AuthRequest]
	tokenRecords    *cache[string, *domain.TokenRecord]
	podSessionByPVC *cache[string, string]

	viewerByPod map[string]map[string]struct{}
}

type ExpiredItem struct {
	Kind string
	ID   string
}

func New(cfg config.CacheConfig) *Store {
	return &Store{
		podSessions:     newCache[string, *domain.PodSession](cfg.PodSessionsMaxEntries),
		viewerSessions:  newCache[string, *domain.ViewerSession](cfg.ViewerSessionsMaxEntries),
		authRequests:    newCache[string, *domain.AuthRequest](cfg.AuthRequestsMaxEntries),
		tokenRecords:    newCache[string, *domain.TokenRecord](cfg.TokenRecordsMaxEntries),
		podSessionByPVC: newCache[string, string](cfg.IndexesMaxEntries),
		viewerByPod:     map[string]map[string]struct{}{},
	}
}

func (s *Store) PutPodSession(session *domain.PodSession) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.podSessions.items[session.ID]; ok {
		oldKey := pvcKey(existing.value.Namespace, existing.value.PVCUID)
		newKey := pvcKey(session.Namespace, session.PVCUID)
		if oldKey != newKey {
			s.podSessionByPVC.delete(oldKey)
		}
	}
	s.podSessions.put(session.ID, clonePodSession(session), session.ExpiresAt)
	s.podSessionByPVC.put(pvcKey(session.Namespace, session.PVCUID), session.ID, session.ExpiresAt)
}

func (s *Store) GetPodSession(id string, now time.Time) (*domain.PodSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.podSessions.get(id, now)
	if !ok {
		return nil, false
	}
	return clonePodSession(session), true
}

func (s *Store) GetPodSessionIncludingExpired(id string) (*domain.PodSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.podSessions.items[id]
	if !ok {
		return nil, false
	}
	return clonePodSession(item.value), true
}

func (s *Store) ListPodSessions(now time.Time) []*domain.PodSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := make([]*domain.PodSession, 0, s.podSessions.len())
	for _, session := range s.podSessions.values(now) {
		sessions = append(sessions, clonePodSession(session))
	}
	return sessions
}

func (s *Store) ListExpiredPodSessions(now time.Time) []*domain.PodSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := make([]*domain.PodSession, 0, s.podSessions.len())
	for _, item := range s.podSessions.items {
		if item.expiresAt.IsZero() || now.Before(item.expiresAt) {
			continue
		}
		sessions = append(sessions, clonePodSession(item.value))
	}
	return sessions
}

func (s *Store) DeletePodSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.podSessions.delete(id); ok {
		s.podSessionByPVC.delete(pvcKey(session.Namespace, session.PVCUID))
	}
	delete(s.viewerByPod, id)
}

func (s *Store) FindPodSessionByPVC(namespace string, pvcUID string, now time.Time) (*domain.PodSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.podSessionByPVC.get(pvcKey(namespace, pvcUID), now)
	if !ok {
		return nil, false
	}
	session, ok := s.podSessions.get(id, now)
	if !ok {
		s.podSessionByPVC.delete(pvcKey(namespace, pvcUID))
		return nil, false
	}
	return clonePodSession(session), true
}

func (s *Store) PutViewerSession(session *domain.ViewerSession) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.viewerSessions.items[session.ID]; ok && existing.value.PodSessionID != session.PodSessionID {
		s.deleteViewerByPodLocked(existing.value.PodSessionID, session.ID)
	}
	s.viewerSessions.put(session.ID, cloneViewerSession(session), session.ExpiresAt)
	if _, ok := s.viewerByPod[session.PodSessionID]; !ok {
		s.viewerByPod[session.PodSessionID] = map[string]struct{}{}
	}
	s.viewerByPod[session.PodSessionID][session.ID] = struct{}{}
}

func (s *Store) GetViewerSession(id string, now time.Time) (*domain.ViewerSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.viewerSessions.get(id, now)
	if !ok {
		return nil, false
	}
	return cloneViewerSession(session), true
}

func (s *Store) DeleteViewerSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.viewerSessions.delete(id)
	if !ok {
		return
	}
	s.deleteViewerByPodLocked(session.PodSessionID, id)
}

func (s *Store) ListViewerSessionsByPod(podSessionID string, now time.Time) []*domain.ViewerSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := s.viewerByPod[podSessionID]
	if len(ids) == 0 {
		return []*domain.ViewerSession{}
	}
	sessions := make([]*domain.ViewerSession, 0, len(ids))
	for id := range ids {
		session, ok := s.viewerSessions.get(id, now)
		if !ok {
			delete(ids, id)
			continue
		}
		sessions = append(sessions, cloneViewerSession(session))
	}
	if len(ids) == 0 {
		delete(s.viewerByPod, podSessionID)
	}
	return sessions
}

func (s *Store) CreateAuthRequest(req *domain.AuthRequest, secret string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	copyReq := cloneAuthRequest(req)
	copyReq.PasswordHash = secret
	s.authRequests.put(copyReq.ID, copyReq, copyReq.ExpiresAt)
}

func (s *Store) ConsumeAuthRequest(id string, passwordHash string, now time.Time) (*domain.AuthRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.authRequests.get(id, now)
	if !ok || req.UsedAt != nil {
		return nil, false
	}
	if subtle.ConstantTimeCompare([]byte(req.PasswordHash), []byte(passwordHash)) != 1 {
		return nil, false
	}
	req.UsedAt = new(now)
	s.authRequests.delete(id)
	return cloneAuthRequest(req), true
}

func (s *Store) PutTokenRecord(record *domain.TokenRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokenRecords.put(record.TokenHash, cloneTokenRecord(record), record.ExpiresAt)
}

func (s *Store) PurgeExpired(now time.Time) []ExpiredItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	expired := make([]ExpiredItem, 0, 4)
	expired = append(expired, purgeCacheLocked("pod_session", s.podSessions, now)...)
	viewerExpired := purgeCacheLocked("viewer_session", s.viewerSessions, now)
	expired = append(expired, viewerExpired...)
	for _, item := range viewerExpired {
		for podSessionID, ids := range s.viewerByPod {
			delete(ids, item.ID)
			if len(ids) == 0 {
				delete(s.viewerByPod, podSessionID)
			}
		}
	}
	expired = append(expired, purgeCacheLocked("auth_request", s.authRequests, now)...)
	expired = append(expired, purgeCacheLocked("token_record", s.tokenRecords, now)...)
	_ = s.podSessionByPVC.purgeExpired(now)
	return expired
}

func purgeCacheLocked[V any](kind string, c *cache[string, V], now time.Time) []ExpiredItem {
	keys := c.purgeExpired(now)
	items := make([]ExpiredItem, 0, len(keys))
	for _, key := range keys {
		items = append(items, ExpiredItem{Kind: kind, ID: key})
	}
	return items
}

func (s *Store) deleteViewerByPodLocked(podSessionID string, viewerSessionID string) {
	ids := s.viewerByPod[podSessionID]
	if len(ids) == 0 {
		return
	}
	delete(ids, viewerSessionID)
	if len(ids) == 0 {
		delete(s.viewerByPod, podSessionID)
	}
}

func pvcKey(namespace string, pvcUID string) string {
	return namespace + "/" + pvcUID
}

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

func (c *cache[K, V]) put(key K, value V, expiresAt time.Time) {
	if item, ok := c.items[key]; ok {
		item.value = value
		item.expiresAt = expiresAt
		c.order.MoveToFront(item.element)
		return
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
			return
		}
		evicted := back.Value.(*cacheItem[K, V])
		delete(c.items, evicted.key)
		c.order.Remove(back)
	}
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

func clonePodSession(session *domain.PodSession) *domain.PodSession {
	if session == nil {
		return nil
	}
	return new(*session)
}

func cloneViewerSession(session *domain.ViewerSession) *domain.ViewerSession {
	if session == nil {
		return nil
	}
	return new(*session)
}

func cloneAuthRequest(req *domain.AuthRequest) *domain.AuthRequest {
	if req == nil {
		return nil
	}
	copyReq := *req
	if req.UsedAt != nil {
		copyReq.UsedAt = new(*req.UsedAt)
	}
	return &copyReq
}

func cloneTokenRecord(record *domain.TokenRecord) *domain.TokenRecord {
	if record == nil {
		return nil
	}
	return new(*record)
}

func (item ExpiredItem) String() string {
	return fmt.Sprintf("%s:%s", item.Kind, item.ID)
}
