package integration

import (
	"fmt"
	"time"

	"github.com/dexon-foundation/dexon-consensus/core/test"
	"github.com/dexon-foundation/dexon-consensus/core/types"
)

// Errors when calculating statistics for events.
var (
	ErrUnknownEvent              = fmt.Errorf("unknown event")
	ErrUnknownConsensusEventType = fmt.Errorf("unknown consensus event type")
)

// StatsSet represents accumulatee result of a group of related events
// (ex. All events from one node).
type StatsSet struct {
	ProposedBlockCount      int
	ReceivedBlockCount      int
	StronglyAckedBlockCount int
	TotalOrderedBlockCount  int
	DeliveredBlockCount     int
	ProposingLatency        time.Duration
	ReceivingLatency        time.Duration
	PrepareExecLatency      time.Duration
	ProcessExecLatency      time.Duration
}

// newBlockProposeEvent accumulates a block proposing event.
func (s *StatsSet) newBlockProposeEvent(
	e *test.Event, payload *consensusEventPayload, history []*test.Event) {

	// Find previous block proposing event.
	if e.ParentHistoryIndex != -1 {
		parentEvent := history[e.ParentHistoryIndex]
		s.ProposingLatency +=
			e.Time.Sub(parentEvent.Time) - parentEvent.ExecInterval
	}
	s.PrepareExecLatency += e.ExecInterval
	s.ProposedBlockCount++
}

// newBlockReceiveEvent accumulates a block received event.
func (s *StatsSet) newBlockReceiveEvent(
	e *test.Event,
	payload *consensusEventPayload,
	history []*test.Event,
	app *test.App) {

	// Find previous block proposing event.
	parentEvent := history[e.ParentHistoryIndex]
	s.ReceivingLatency +=
		e.Time.Sub(parentEvent.Time) - parentEvent.ExecInterval
	s.ProcessExecLatency += e.ExecInterval
	s.ReceivedBlockCount++

	// Find statistics from test.App
	block := payload.PiggyBack.(*types.Block)
	app.Check(func(app *test.App) {
		// Is this block strongly acked?
		if _, exists := app.Acked[block.Hash]; !exists {
			return
		}
		s.StronglyAckedBlockCount++

		// Is this block total ordered?
		if _, exists := app.TotalOrderedByHash[block.Hash]; !exists {
			return
		}
		s.TotalOrderedBlockCount++

		// Is this block delivered?
		if _, exists := app.Delivered[block.Hash]; !exists {
			return
		}
		s.DeliveredBlockCount++
	})
}

// done would divide the latencies we cached with related event count. This way
// to calculate average latency is more accurate.
func (s *StatsSet) done(nodeCount int) {
	s.ProposingLatency /= time.Duration(s.ProposedBlockCount - nodeCount)
	s.ReceivingLatency /= time.Duration(s.ReceivedBlockCount)
	s.PrepareExecLatency /= time.Duration(s.ProposedBlockCount)
	s.ProcessExecLatency /= time.Duration(s.ReceivedBlockCount)
}

// Stats is statistics of a slice of test.Event generated by nodes.
type Stats struct {
	ByNode        map[types.NodeID]*StatsSet
	All           *StatsSet
	BPS           float64
	ExecutionTime time.Duration
}

// NewStats constructs an Stats instance by providing a slice of
// test.Event.
func NewStats(
	history []*test.Event, apps map[types.NodeID]*test.App) (
	stats *Stats, err error) {

	stats = &Stats{
		ByNode: make(map[types.NodeID]*StatsSet),
		All:    &StatsSet{},
	}
	if err = stats.calculate(history, apps); err != nil {
		stats = nil
	}
	stats.summary(history)
	return
}

func (stats *Stats) calculate(
	history []*test.Event, apps map[types.NodeID]*test.App) error {

	defer func() {
		stats.All.done(len(stats.ByNode))
		for _, set := range stats.ByNode {
			set.done(1)
		}
	}()

	for _, e := range history {
		payload, ok := e.Payload.(*consensusEventPayload)
		if !ok {
			return ErrUnknownEvent
		}
		switch payload.Type {
		case evtProposeBlock:
			stats.All.newBlockProposeEvent(
				e, payload, history)
			stats.getStatsSetByNode(e.NodeID).newBlockProposeEvent(
				e, payload, history)
		case evtReceiveBlock:
			stats.All.newBlockReceiveEvent(
				e, payload, history, apps[e.NodeID])
			stats.getStatsSetByNode(e.NodeID).newBlockReceiveEvent(
				e, payload, history, apps[e.NodeID])
		default:
			return ErrUnknownConsensusEventType
		}
	}
	return nil
}

func (stats *Stats) getStatsSetByNode(
	vID types.NodeID) (s *StatsSet) {

	s = stats.ByNode[vID]
	if s == nil {
		s = &StatsSet{}
		stats.ByNode[vID] = s
	}
	return
}

func (stats *Stats) summary(history []*test.Event) {
	// Find average delivered block count among all blocks.
	totalConfirmedBlocks := 0
	for _, s := range stats.ByNode {
		totalConfirmedBlocks += s.DeliveredBlockCount
	}
	averageConfirmedBlocks := totalConfirmedBlocks / len(stats.ByNode)

	// Find execution time.
	// Note: it's a simplified way to calculate the execution time:
	//       the latest event might not be at the end of history when
	//       the number of worker routine is larger than 1.
	stats.ExecutionTime = history[len(history)-1].Time.Sub(history[0].Time)
	// Calculate BPS.
	latencyAsSecond := stats.ExecutionTime.Nanoseconds() / (1000 * 1000 * 1000)
	stats.BPS = float64(averageConfirmedBlocks) / float64(latencyAsSecond)
}
