package fanoffset

type learnedMemoryStore struct {
	byKey map[learnedMemoryKey]learnedState
}

func newLearnedMemoryStore() *learnedMemoryStore {
	return &learnedMemoryStore{byKey: make(map[learnedMemoryKey]learnedState)}
}

func (s *learnedMemoryStore) ensure() {
	if s.byKey == nil {
		s.byKey = make(map[learnedMemoryKey]learnedState)
	}
}

func (s *learnedMemoryStore) Get(curveSignature string, zoneTemp int) (learnedState, bool) {
	if s == nil {
		return learnedState{}, false
	}
	state, ok := s.byKey[learnedMemoryKey{curveSignature: curveSignature, zoneTemp: zoneTemp}]
	return state, ok
}

func (s *learnedMemoryStore) Set(curveSignature string, zoneTemp int, state learnedState) {
	if s == nil {
		return
	}
	s.ensure()
	s.byKey[learnedMemoryKey{curveSignature: curveSignature, zoneTemp: zoneTemp}] = state
}

func (s *learnedMemoryStore) Delete(curveSignature string, zoneTemp int) {
	if s == nil {
		return
	}
	delete(s.byKey, learnedMemoryKey{curveSignature: curveSignature, zoneTemp: zoneTemp})
}

func (s *learnedMemoryStore) Len() int {
	if s == nil {
		return 0
	}
	return len(s.byKey)
}

func (s *learnedMemoryStore) CountForCurve(curveSignature string) int {
	if s == nil {
		return 0
	}
	count := 0
	for key := range s.byKey {
		if key.curveSignature == curveSignature {
			count++
		}
	}
	return count
}

func (s *learnedMemoryStore) HasSeedForCurve(curveSignature string) bool {
	if s == nil {
		return false
	}
	for key, state := range s.byKey {
		if key.curveSignature == curveSignature && state.hasSeed {
			return true
		}
	}
	return false
}
