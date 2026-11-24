package sunspec

// ChainSegment is one device in a SunSpec chain: the Common model that opens
// it and every model that follows it until the next Common model.
type ChainSegment struct {
	occurrences []Occurrence
}

// Occurrences returns copies of the segment's models in chain order, the
// opening Common model first.
func (s ChainSegment) Occurrences() []Occurrence {
	out := make([]Occurrence, len(s.occurrences))
	for index, occurrence := range s.occurrences {
		out[index] = cloneOccurrence(occurrence)
	}
	return out
}

// SegmentChain returns one segment per Common model in snapshot. Each segment
// holds that Common model and the models that follow it until the next Common
// model, so an inverter and a meter in one chain are two devices. Occurrences
// before the first Common model are not a segment. The end marker is not an
// occurrence and is not part of a segment.
func SegmentChain(snapshot ChainSnapshot) []ChainSegment {
	var segments []ChainSegment
	var current []Occurrence
	for _, occurrence := range snapshot.Occurrences() {
		if occurrence.ModelID() != 1 {
			if current != nil {
				current = append(current, occurrence)
			}
			continue
		}
		if current != nil {
			segments = append(segments, ChainSegment{occurrences: current})
		}
		current = []Occurrence{occurrence}
	}
	if current != nil {
		segments = append(segments, ChainSegment{occurrences: current})
	}
	return segments
}
