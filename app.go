package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"gopkg.in/CodapeWild/dd-trace-go.v1/ddtrace/tracer"
)

type tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type span struct {
	Name           string  `json:"name"`
	Duration       int64   `json:"duration"`
	Tags           []tag   `json:"tags"`
	Simultaneously bool    `json:"simultaneously"`
	Children       []*span `json:"children"`
}

type config struct {
	DkAgent      string  `json:"dk_agent"`
	Service      string  `json:"service"`
	CallingChain []*span `json:"calling_chain"`
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	f, err := os.OpenFile("./config.json", os.O_RDONLY, 0755)
	if err != nil {
		log.Fatalln(err.Error())
	}

	config := &config{}
	if err = json.NewDecoder(f).Decode(config); err != nil {
		log.Fatalln(err.Error())
	}

	tracer.Start(tracer.WithAgentAddr(config.DkAgent), tracer.WithService(config.Service), tracer.WithDebugMode(true), tracer.WithLogStartup(true))
	defer tracer.Stop()

	m := 1
	wg := &sync.WaitGroup{}
	wg.Add(m)

	for j := 0; j < m; j++ {
		go func() {
			span := tracer.StartSpan("root")
			defer func() {
				span.Finish()
				tracer.Flush()
				wg.Done()
			}()

			orchestrator(tracer.ContextWithSpan(context.Background(), span), config.CallingChain)
		}()
		time.Sleep(time.Millisecond)
	}
	wg.Wait()
}

func orchestrator(ctx context.Context, spans []*span) {
	var sync, async []*span
	for k := range spans {
		if spans[k].Simultaneously {
			sync = append(sync, spans[k])
		} else {
			async = append(async, spans[k])
		}
	}
	for k := range sync {
		go startSpanFromContext(ctx, sync[k])
	}
	for k := range async {
		startSpanFromContext(ctx, async[k])
	}
}

func startSpanFromContext(ctx context.Context, span *span) context.Context {
	ddspan, ctx := tracer.StartSpanFromContext(ctx, span.Name)
	defer ddspan.Finish()

	for k := range span.Tags {
		ddspan.SetTag(span.Tags[k].Key, span.Tags[k].Value)
	}
	time.Sleep(time.Duration(span.Duration * time.Second.Milliseconds()))

	if len(span.Children) != 0 {
		orchestrator(ctx, span.Children)
	}

	return ctx
}
