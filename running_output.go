package main

import (
	"sync"
	"log"
	"time"
)

const (
	// Default size of metrics batch size.
	DEFAULT_METRIC_BATCH_SIZE = 1000

	// Default number of metrics kept. It should be a multiple of batch size.
	DEFAULT_METRIC_BUFFER_LIMIT = 10000
)

// RunningOutput contains the output configuration
type RunningOutput struct {
	Name   string
	Output Output
	Config *OutputConfig
	MetricBufferLimit int
	MetricBatchSize   int

	MetricsWritten  Stat
	BufferSize      Stat
	WriteTime       Stat

	metrics     *Buffer
	failMetrics *Buffer

	// Guards against concurrent calls to the Output as described in #3009
	sync.Mutex
}

func NewRunningOutput(
	name string,
	output Output,
	conf *OutputConfig,
) *RunningOutput {
	ro := &RunningOutput{
		Name:   name,
		Output: output,
		Config: conf,
	}
	return ro
}

// Write writes all cached points to this output.
func (ro *RunningOutput) Write() error {
	nFails, nMetrics := ro.failMetrics.Len(), ro.metrics.Len()
	ro.BufferSize.Set(int64(nFails + nMetrics))
	log.Printf("D! Output [%s] buffer fullness: %d / %d metrics. ",
		ro.Name, nFails+nMetrics, ro.MetricBufferLimit)
	var err error
	if !ro.failMetrics.IsEmpty() {
		// how many batches of failed writes we need to write.
		nBatches := nFails/ro.MetricBatchSize + 1
		batchSize := ro.MetricBatchSize

		for i := 0; i < nBatches; i++ {
			// If it's the last batch, only grab the metrics that have not had
			// a write attempt already (this is primarily to preserve order).
			if i == nBatches-1 {
				batchSize = nFails % ro.MetricBatchSize
			}
			batch := ro.failMetrics.Batch(batchSize)
			// If we've already failed previous writes, don't bother trying to
			// write to this output again. We are not exiting the loop just so
			// that we can rotate the metrics to preserve order.
			if err == nil {
				err = ro.write(batch)
			}
			if err != nil {
				ro.failMetrics.Add(batch...)
			}
		}
	}

	batch := ro.metrics.Batch(ro.MetricBatchSize)
	// see comment above about not trying to write to an already failed output.
	// if ro.failMetrics is empty then err will always be nil at this point.
	if err == nil {
		err = ro.write(batch)
	}

	if err != nil {
		ro.failMetrics.Add(batch...)
		return err
	}
	return nil
}

func (ro *RunningOutput) write(metrics []Metric) error {
	nMetrics := len(metrics)
	if nMetrics == 0 {
		return nil
	}
	ro.Lock()
	defer ro.Unlock()
	start := time.Now()
	err := ro.Output.Write(metrics)
	elapsed := time.Since(start)
	if err == nil {
		log.Printf("D! Output [%s] wrote batch of %d metrics in %s\n",
			ro.Name, nMetrics, elapsed)
		ro.MetricsWritten.Incr(int64(nMetrics))
		ro.WriteTime.Incr(elapsed.Nanoseconds())
	}
	return err
}

// OutputConfig containing name and filter
type OutputConfig struct {
	Name string
}

// AddMetric adds a metric to the output. This function can also write cached
// points if FlushBufferWhenFull is true.
func (ro *RunningOutput) AddMetric(m Metric) {
	if m == nil {
		return
	}

	ro.metrics.Add(m)
	if ro.metrics.Len() == ro.MetricBatchSize {
		batch := ro.metrics.Batch(ro.MetricBatchSize)
		err := ro.write(batch)
		if err != nil {
			ro.failMetrics.Add(batch...)
		}
	}
}