package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace"
	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace/tracer"
)

type config struct {
	DkAgent   string  `json:"dk_agent"`
	Threads   int     `json:"threads"`
	SendCount int     `json:"send_count"`
	Service   string  `json:"service"`
	Trace     []*span `json:"trace"`
}

type tag struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

type span struct {
	Resource  string  `json:"resource"`
	Operation string  `json:"operation"`
	SpanType  string  `json:"span_type"`
	Duration  int64   `json:"duration"`
	Error     string  `json:"error"`
	Tags      []tag   `json:"tags"`
	Children  []*span `json:"children"`
}

func main() {
	data, err := os.ReadFile("./config.json")
	if err != nil {
		log.Fatalln(err.Error())
	}

	config := &config{}
	if err = json.Unmarshal(data, config); err != nil {
		log.Fatalln(err.Error())
	}

	tracer.Start(tracer.WithAgentAddr(config.DkAgent), tracer.WithService(config.Service), tracer.WithDebugMode(true), tracer.WithLogStartup(true))
	defer tracer.Stop()

	root, children := startRootSpan(config.Trace)

	orchestrator(tracer.ContextWithSpan(context.Background(), root), children)
}

func startRootSpan(trace []*span) (root ddtrace.Span, children []*span) {
	var (
		d   int64
		err error
	)
	if len(trace) == 1 {
		root = tracer.StartSpan(trace[0].Operation)
		root.SetTag(ext.ResourceName, trace[0].Resource)
		root.SetTag(ext.SpanType, trace[0].SpanType)
		for _, tag := range trace[0].Tags {
			root.SetTag(tag.Key, tag.Value)
		}
		d = trace[0].Duration * time.Hour.Milliseconds()
		children = trace[0].Children
		if len(trace[0].Error) != 0 {
			err = errors.New(trace[0].Error)
		}
	} else {
		root = tracer.StartSpan("start_root_span")
		d = int64(time.Second)
		children = trace
	}
	time.Sleep(time.Duration(d))
	root.Finish(tracer.WithError(err))

	return
}

func orchestrator(ctx context.Context, trace []*span) {
	if len(trace) == 1 {
		ctx = startSpanFromContext(ctx, trace[0])
		if len(trace[0].Children) != 0 {
			orchestrator(ctx, trace[0].Children)
		}
	} else {
		for _, v := range trace {
			go func(ctx context.Context, span *span) {
				ctx = startSpanFromContext(ctx, span)
				if len(span.Children) != 0 {
					orchestrator(ctx, span.Children)
				}
			}(ctx, v)
		}
	}
}

func startSpanFromContext(ctx context.Context, span *span) context.Context {
	ddspan, ctx := tracer.StartSpanFromContext(ctx, span.Operation)

	ddspan.SetTag(ext.ResourceName, span.Resource)
	ddspan.SetTag(ext.SpanType, span.SpanType)
	for _, tag := range span.Tags {
		ddspan.SetTag(tag.Key, tag.Value)
	}

	var err error
	if len(span.Error) != 0 {
		err = errors.New(span.Error)
	}

	time.Sleep(time.Duration(span.Duration * time.Second.Milliseconds()))

	ddspan.Finish(tracer.WithError(err))

	return ctx
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
