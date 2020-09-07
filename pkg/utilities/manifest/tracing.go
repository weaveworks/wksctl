package manifest

import (
	"bytes"
	"io"

	ot "github.com/opentracing/opentracing-go"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"sigs.k8s.io/controller-runtime/pkg/tracing"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

func WithTraceAnnotation(rc io.ReadCloser, span ot.Span) ([]byte, error) {
	fr := serializer.NewYAMLFrameReader(rc)
	buf := new(bytes.Buffer)
	fw := serializer.NewYAMLFrameWriter(buf)

	spanContext, err := tracing.GenerateEmbeddableSpanContext(span)
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
		if err := obj.PipeE(yaml.SetAnnotation(tracing.TraceAnnotationKey, spanContext)); err != nil {
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
