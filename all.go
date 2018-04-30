package main

import (
)

func InitAllInputs() {
	AddInput("cpu", func() Input {
		return &CPUStats{
			PerCPU:   true,
			TotalCPU: true,
			ps:       nil,
		}
	})

	AddInput("mem", func() Input {
		return &MemStats{ps: nil}
	})
}

func InitAllOutputs() {
	AddOutput("influxdb", func() Output { return newInflux() })
}
