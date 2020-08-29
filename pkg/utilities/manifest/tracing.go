package manifest

import (
	"bytes"
	"encoding/base64"
	"io"

	ot "github.com/opentracing/opentracing-go"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const TraceAnnotationKey string = "trace.kubernetes.io/context"

// generateEmbeddableSpanContext takes a Span and returns a serialized string
func generateEmbeddableSpanContext(span ot.Span) (string, error) {
	var buf bytes.Buffer
	if err := ot.GlobalTracer().Inject(span.Context(), ot.Binary, &buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func SpanFromAnnotations(name string, annotations map[string]string) (ot.Span, error) {
	value, found := annotations[TraceAnnotationKey]
	if !found {
		return nil, nil
	}
	return spanFromEmbeddableSpanContext(name, value)
}

func spanFromEmbeddableSpanContext(name, value string) (ot.Span, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	spanContext, err := ot.GlobalTracer().Extract(ot.Binary, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	return ot.StartSpan(name, ot.FollowsFrom(spanContext)), nil
}

func WithTraceAnnotation(rc io.ReadCloser, span ot.Span) ([]byte, error) {
	fr := serializer.NewYAMLFrameReader(rc)
	buf := new(bytes.Buffer)
	fw := serializer.NewYAMLFrameWriter(buf)

	spanContext, err := generateEmbeddableSpanContext(span)
	if err != nil {
		return nil, err
	}

	// Read all frames from the FrameReader
	frames, err := serializer.ReadFrameList(fr)
	if err != nil {
		return nil, err
	}

	for _, frame := range frames {
		obj, err := kyaml.Parse(string(frame))
		if err != nil {
			return nil, err
		}
		if err := obj.PipeE(yaml.SetAnnotation(TraceAnnotationKey, spanContext)); err != nil {
			return nil, err
		}
		// Convert the object to string, and write it to the FrameWriter
		str, err := obj.String()
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write([]byte(str)); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
