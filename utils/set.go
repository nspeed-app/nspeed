package utils

// a mix of  https://gist.github.com/bgadrian/cb8b9344d9c66571ef331a14eb7a2e80
// and https://bitfieldconsulting.com/golang/generic-set

type Set[T comparable] struct {
	list map[T]struct{}
}

func NewSet[T comparable](vals ...T) *Set[T] {
	s := &Set[T]{}
	s.list = make(map[T]struct{})
	for _, v := range vals {
		s.list[v] = struct{}{}
	}
	return s
}

func (s *Set[T]) Has(v T) bool {
	_, ok := s.list[v]
	return ok
}

func (s *Set[E]) Add(vals ...E) {
	for _, v := range vals {
		s.list[v] = struct{}{}
	}
}

func (s *Set[T]) Remove(v T) {
	delete(s.list, v)
}

func (s *Set[T]) Clear() {
	s.list = make(map[T]struct{})
}

func (s *Set[T]) Size() int {
	return len(s.list)
}

//optional functionalities

// AddMulti Add multiple values in the set
func (s *Set[T]) AddMulti(list ...T) {
	for _, v := range list {
		s.Add(v)
	}
}

type FilterFunc[T any] func(v T) bool

// Filter returns a subset, that contains only the values that satisfies the given predicate P
func (s *Set[T]) Filter(P FilterFunc[T]) *Set[T] {
	res := NewSet[T]()
	for v := range s.list {
		if P(v) == false {
			continue
		}
		res.Add(v)
	}
	return res
}

func (s *Set[T]) Union(s2 *Set[T]) *Set[T] {
	res := NewSet[T]()
	for v := range s.list {
		res.Add(v)
	}

	for v := range s2.list {
		res.Add(v)
	}
	return res
}

func (s *Set[T]) Intersect(s2 *Set[T]) *Set[T] {
	res := NewSet[T]()
	for v := range s.list {
		if s2.Has(v) == false {
			continue
		}
		res.Add(v)
	}
	return res
}

// Difference returns the subset from s, that doesn't exists in s2 (param)
func (s *Set[T]) Difference(s2 *Set[T]) *Set[T] {
	res := NewSet[T]()
	for v := range s.list {
		if s2.Has(v) {
			continue
		}
		res.Add(v)
	}
	return res
}

// range
func (s *Set[T]) Iter() []T {
	var keys []T
	for k := range s.list {
		keys = append(keys, k)
	}
	return keys
}
