# DDTrace Agent for Datakit

**Notice:** THIS PROJECT IS STILL IN PROGRESS

This tool used to send standard DDTrace tracing data to Datakit.

The features include:

- build with standard DDTrace Golang lib
- customized Span data
- configurable multi-thread pressure test

## Config

```json
{
  "dk_agent": "127.0.0.1:9529",
  "sender": {
    "threads": 1,
    "send_count": 1
  },
  "service": "dktrace-dd-agent",
  "trace": []
}
```

- `dk_agent`: Datakit host address
- `sender.threads`: how many threads will start to send `trace` simultaneously
- `sender.send_count`: how many times `trace` will be send in one `thread`
- `service`: service name
- `trace`: represents a Trace consists of Spans

## Span Structure

`trace`\[span...\]

```json
{
  "resource": "/get/user/name",
  "operation": "user.getUserName",
  "span_type": "",
  "duration": 1000,
  "error": "",
  "tags": [],
  "children": []
}
```

**Note:** Spans list in `trace` or `children` will generate concurrency Spans, nested in `trace` or `children` will show up as calling chain.

- `resource`: resource name
- `operation`: operation name
- `span_type`: Span type [app cache custom db web]
- `duration`: how long an operation process will last
- `error`: error string
- `tags`: Span meta data used to set [DDTrace Tags](#standard-ddtrace-tags)
- `children`: child Spans represent a subsequent function calling from current `operation`

## Standard DDTrace Tags

```golang
// Package ext contains a set of Datadog-specific constants. Most of them are used
// for setting span metadata.
package ext

const (
	// TargetHost sets the target host address.
	TargetHost = "out.host"

	// TargetPort sets the target host port.
	TargetPort = "out.port"

	// SamplingPriority is the tag that marks the sampling priority of a span.
	SamplingPriority = "sampling.priority"

	// SQLType sets the sql type tag.
	SQLType = "sql"

	// SQLQuery sets the sql query tag on a span.
	SQLQuery = "sql.query"

	// HTTPMethod specifies the HTTP method used in a span.
	HTTPMethod = "http.method"

	// HTTPCode sets the HTTP status code as a tag.
	HTTPCode = "http.status_code"

	// HTTPURL sets the HTTP URL for a span.
	HTTPURL = "http.url"

	// SpanName is a pseudo-key for setting a span's operation name by means of
	// a tag. It is mostly here to facilitate vendor-agnostic frameworks like Opentracing
	// and OpenCensus.
	SpanName = "span.name"

	// SpanType defines the Span type (web, db, cache).
	SpanType = "span.type"

	// ServiceName defines the Service name for this Span.
	ServiceName = "service.name"

	// Version is a tag that specifies the current application version.
	Version = "version"

	// ResourceName defines the Resource name for the Span.
	ResourceName = "resource.name"

	// Error specifies the error tag. It's value is usually of type "error".
	Error = "error"

	// ErrorMsg specifies the error message.
	ErrorMsg = "error.msg"

	// ErrorType specifies the error type.
	ErrorType = "error.type"

	// ErrorStack specifies the stack dump.
	ErrorStack = "error.stack"

	// ErrorDetails holds details about an error which implements a formatter.
	ErrorDetails = "error.details"

	// Environment specifies the environment to use with a trace.
	Environment = "env"

	// EventSampleRate specifies the rate at which this span will be sampled
	// as an APM event.
	EventSampleRate = "_dd1.sr.eausr"

	// AnalyticsEvent specifies whether the span should be recorded as a Trace
	// Search & Analytics event.
	AnalyticsEvent = "analytics.event"

	// ManualKeep is a tag which specifies that the trace to which this span
	// belongs to should be kept when set to true.
	ManualKeep = "manual.keep"

	// ManualDrop is a tag which specifies that the trace to which this span
	// belongs to should be dropped when set to true.
	ManualDrop = "manual.drop"

	// RuntimeID is a tag that contains a unique id for this process.
	RuntimeID = "runtime-id"
)
```

**_more tags maybe usefull_**

- \_dd.origin
