package fanoffset

import (
	"strconv"
	"strings"
)

type learnedMemoryCandidate struct {
	curveSignature string
	rpm            int
	state          learnedState
	rpmGap         int
}

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

func (s *learnedMemoryStore) BestAlternativeForZone(currentCurveSignature string, zoneTemp, targetRPM int, minConfidence float64) (learnedMemoryCandidate, bool) {
	if s == nil {
		return learnedMemoryCandidate{}, false
	}
	best := learnedMemoryCandidate{}
	bestFound := false
	for key, state := range s.byKey {
		if key.curveSignature == currentCurveSignature || key.zoneTemp != zoneTemp {
			continue
		}
		if !state.hasSeed || state.confidence < minConfidence {
			continue
		}
		rpm, ok := rpmFromCurveSignature(key.curveSignature, zoneTemp)
		if !ok {
			continue
		}
		gap := targetRPM - rpm
		if gap < 0 {
			gap = -gap
		}
		candidate := learnedMemoryCandidate{
			curveSignature: key.curveSignature,
			rpm:            rpm,
			state:          state,
			rpmGap:         gap,
		}
		if !bestFound || candidate.rpmGap < best.rpmGap || (candidate.rpmGap == best.rpmGap && candidate.state.confidence > best.state.confidence) {
			best = candidate
			bestFound = true
		}
	}
	return best, bestFound
}

func rpmFromCurveSignature(curveSignature string, zoneTemp int) (int, bool) {
	if strings.TrimSpace(curveSignature) == "" {
		return 0, false
	}
	zoneKey := strconv.Itoa(zoneTemp)
	for _, segment := range strings.Split(curveSignature, "|") {
		parts := strings.SplitN(segment, ":", 2)
		if len(parts) != 2 || parts[0] != zoneKey {
			continue
		}
		rpm, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, false
		}
		return rpm, true
	}
	return 0, false
}
