package gossip

import (
	"errors"
	"math/rand"
)

// Node models a participant with a list of peer IDs.
type Node struct {
	ID    string
	Peers []string
}

// Result captures propagation outcomes.
type Result struct {
	Steps        int
	ReachedCount int
	ReachedRatio float64
	Total        int
}

// Simulator runs an epidemic broadcast simulation with a deterministic RNG.
type Simulator struct {
	rng *rand.Rand
}

// NewSimulator constructs a simulator with the provided seed.
func NewSimulator(seed int64) *Simulator {
	return &Simulator{rng: rand.New(rand.NewSource(seed))}
}

// SimulatePropagation spreads a payload from initialID using fanout per step.
func (s *Simulator) SimulatePropagation(nodes []Node, initialID string, fanout int, targetRatio float64, maxSteps int) (Result, error) {
	if s == nil || s.rng == nil {
		return Result{}, errors.New("simulator is nil")
	}
	if len(nodes) == 0 {
		return Result{}, errors.New("no nodes to simulate")
	}
	if fanout <= 0 {
		return Result{}, errors.New("fanout must be positive")
	}
	if targetRatio <= 0 || targetRatio > 1 {
		return Result{}, errors.New("target ratio must be in (0,1]")
	}
	if maxSteps <= 0 {
		return Result{}, errors.New("max steps must be positive")
	}

	peersByID := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		peersByID[node.ID] = node.Peers
	}
	if _, ok := peersByID[initialID]; !ok {
		return Result{}, errors.New("initial node not found")
	}

	infected := map[string]bool{initialID: true}
	total := len(nodes)

	for step := 1; step <= maxSteps; step++ {
		newlyInfected := make(map[string]bool)
		for nodeID := range infected {
			peers := peersByID[nodeID]
			selected := selectRandomPeers(peers, fanout, s.rng)
			for _, peerID := range selected {
				if infected[peerID] {
					continue
				}
				newlyInfected[peerID] = true
			}
		}

		for nodeID := range newlyInfected {
			infected[nodeID] = true
		}

		reached := len(infected)
		reachedRatio := float64(reached) / float64(total)
		if reachedRatio >= targetRatio {
			return Result{Steps: step, ReachedCount: reached, ReachedRatio: reachedRatio, Total: total}, nil
		}
	}

	reached := len(infected)
	return Result{Steps: maxSteps, ReachedCount: reached, ReachedRatio: float64(reached) / float64(total), Total: total}, errors.New("target ratio not reached")
}

func selectRandomPeers(peers []string, fanout int, rng *rand.Rand) []string {
	if len(peers) == 0 {
		return nil
	}
	if fanout >= len(peers) {
		result := make([]string, len(peers))
		copy(result, peers)
		return result
	}

	indices := rng.Perm(len(peers))
	selected := make([]string, 0, fanout)
	for i := 0; i < fanout; i++ {
		selected = append(selected, peers[indices[i]])
	}
	return selected
}
