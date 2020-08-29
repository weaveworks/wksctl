package manifest

import (
	"log"
	"testing"

	ot "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"

	"github.com/weaveworks/wksctl/utilities/tracing"
)

func TestInjectExtract(t *testing.T) {
	tracingCloser, err := tracing.SetupJaeger("existingInfra-controller")
	if err != nil {
		log.Fatalf("failed to set up Jaeger: %v", err)
	}
	defer tracingCloser.Close()

	sp := ot.StartSpan("foo")
	val, err := generateEmbeddableSpanContext(sp)
	assert.NoError(t, err)
	sp2, err := spanFromEmbeddableSpanContext("bar", val)
	assert.NoError(t, err)
	assert.NotNil(t, sp2)
}
