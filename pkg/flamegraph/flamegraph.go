package flamegraph

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

// FlameGraph is a Go facility to execute stack trace visualizer FlameGraph
// and get a string with the resulting svg graph back.
// Learn more about FlameGraph: https://github.com/brendangregg/flamegraph
type FlameGraph struct {
	stackCollapseBinaryPath string
	flameGraphBinaryPath    string
}

// New creates a new instance of FlameGraph,
// It will need the stackcollapse-bpftrace.pl script and the flamegraph.pl script
// stackcollapse-bpftrace.pl -> https://github.com/brendangregg/FlameGraph/blob/1b1c6deede9c33c5134c920bdb7a44cc5528e9a7/stackcollapse-bpftrace.pl
// flamegraph.pl 			 -> https://github.com/brendangregg/FlameGraph/blob/1b1c6deede9c33c5134c920bdb7a44cc5528e9a7/flamegraph.pl
func New(stackCollapseBinaryPath, flameGraphBinaryPath string) FlameGraph {
	return FlameGraph{
		stackCollapseBinaryPath: stackCollapseBinaryPath,
		flameGraphBinaryPath:    flameGraphBinaryPath,
	}
}

// Generate generates the actual FlameGraph from a stackFileReader
func (o FlameGraph) Generate(ctx context.Context, stackFileReader io.Reader) (io.ReadWriter, error) {
	errBuf := new(bytes.Buffer)
	stackBuf := new(bytes.Buffer)
	c := exec.CommandContext(ctx, o.stackCollapseBinaryPath, "/dev/stdin")
	c.Stdout = stackBuf
	c.Stderr = errBuf
	c.Stdin = stackFileReader
	if err := c.Run(); err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)

	flc := exec.CommandContext(ctx, o.flameGraphBinaryPath, "--title=kubectl-trace FlameGraph", "/dev/stdin")
	flc.Stdin = stackBuf
	flc.Stdout = buf
	c.Stderr = errBuf
	if err := flc.Run(); err != nil {
		return nil, err
	}

	return buf, nil
}
