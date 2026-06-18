package agentkit

import "iter"

// Stream is the incremental result of one Send. It is drained exactly once.
type Stream struct {
	run    func(yield func(Event) bool) (bool, error)
	onDone func(success bool)

	started bool
	done    bool

	err      error
	usage    Usage
	warnings []Warning
	cost     Cost
}

func errorStream(err error) *Stream {
	return &Stream{
		done: true,
		err:  err,
	}
}

// Events yields each event of the turn in order until completion or failure.
func (s *Stream) Events() iter.Seq[Event] {
	return func(yield func(Event) bool) {
		if s == nil || s.started {
			return
		}
		s.started = true

		success := s.err == nil
		if s.run != nil {
			var err error
			success, err = s.run(yield)
			s.err = err
		}
		s.done = true
		if s.onDone != nil {
			s.onDone(success && s.err == nil)
		}
	}
}

// Err returns the terminal error of the turn, or nil.
func (s *Stream) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}

// Usage returns the token usage for the turn.
func (s *Stream) Usage() Usage {
	if s == nil {
		return Usage{}
	}
	return s.usage
}

// Warnings returns provider degradation warnings for the turn.
func (s *Stream) Warnings() []Warning {
	if s == nil {
		return nil
	}
	return append([]Warning(nil), s.warnings...)
}

// Cost returns this turn's cost.
func (s *Stream) Cost() Cost {
	if s == nil {
		return 0
	}
	return s.cost
}
