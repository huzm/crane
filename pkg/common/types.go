package common

import "fmt"

// TimeSeries is a stream of samples that belong to a metric with a set of labels
type TimeSeries struct {
	// A collection of Labels that are attached by monitoring system as metadata
	// for the metrics, which are known as dimensions.
	Labels []Label
	// A collection of Samples in chronological order.
	Samples []Sample
}

// Sample pairs a Value with a Timestamp.
type Sample struct {
	Value     float64
	Timestamp int64
}

// A Label is a Name and Value pair that provides additional information about the metric.
// It is metadata for the metric. For example, Kubernetes pod metrics always have
// 'namespace' label that represents which namespace the pod belongs to.
type Label struct {
	Name  string
	Value string
}

func (s *Sample) String() string {
	return fmt.Sprintf("%d %f", s.Timestamp, s.Value)
}

func (l *Label) String() string {
	return l.Name + "=" + l.Value
}

func (ts *TimeSeries) SetLabels(labels []Label) {
	ts.Labels = labels
}

func (ts *TimeSeries) SetSamples(samples []Sample) {
	ts.Samples = samples
}

func (ts *TimeSeries) AppendLabel(key, val string) {
	ts.Labels = append(ts.Labels, Label{key, val})
}

func (ts *TimeSeries) AppendSample(timestamp int64, val float64) {
	ts.Samples = append(ts.Samples, Sample{Timestamp: timestamp, Value: val})
}

func NewTimeSeries() *TimeSeries {
	return &TimeSeries{
		Labels:  make([]Label, 0),
		Samples: make([]Sample, 0),
	}
}

// QueryCondition is a key, operator, value triple.
// E.g. 'namespace = default', 'role in [Admin, Developer]'
type QueryCondition struct {
	Key      string
	Operator Operator
	Value    []string
}

type Operator string

const (
	OperatorEqual         Operator = "="
	OperatorNotEqual      Operator = "!="
	OperatorRegexMatch    Operator = "=~"
	OperatorNotRegexMatch Operator = "!~"
	OperatorIn            Operator = "in"
)

type Options map[string]interface{}
