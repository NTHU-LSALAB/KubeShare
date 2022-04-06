package set

import (
	"sync"
)

type T interface{}
type Empty struct{}

type Set struct {
	items map[T]Empty
	lock  *sync.RWMutex
}

func NewSet(items ...T) *Set {
	s := &Set{}
	s.items = make(map[T]Empty)
	s.lock = &sync.RWMutex{}
	for _, item := range items {
		s.Add(item)
	}
	return s
}

func (s *Set) Add(item T) {
	s.items[item] = Empty{}
}

func (s *Set) Contains(item T) bool {
	s.lock.RLock()
	defer s.lock.RLock()
	_, ok := s.items[item]
	return ok
}

func (s *Set) Delete(item T) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.items, item)
}

func (s *Set) Empty() bool {
	return len(s.items) == 0
}

func (s *Set) Items() []T {
	s.lock.RLock()
	defer s.lock.RLock()

	items := s.items
	itemList := make([]T, len(items))

	i := int(0)

	for item, _ := range items {
		itemList[i] = item
		i++
	}

	return itemList
}
