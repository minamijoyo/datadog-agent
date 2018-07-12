package custommetrics

import "testing"

type mockProcessor struct {
	sink chan PodMetricValue
}

func newMockProcessor() *mockProcessor {
	return &mockProcessor{
		sink: make(chan PodMetricValue, 1),
	}
}

func (m *mockProcessor) Start() {}

func (m *mockProcessor) Process(podMetric PodMetricValue) {
	m.sink <- podMetric
}

func (m *mockProcessor) Stop() error { return nil }

func TestBufferedProcessor(t *testing.T) {
	// TODO
}
