package tracing

import (
	"context"

	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewRuntimeClient creates a controller-runtime Client which wraps every call in an OpenTracing span.
func NewRuntimeClient(cache cache.Cache, config *rest.Config, options client.Options) (client.Client, error) {
	// initial code copied from defaultNewClient()
	// Create the Client for Write operations.
	c, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	delegatingClient := &client.DelegatingClient{
		Reader: &client.DelegatingReader{
			CacheReader:  cache,
			ClientReader: c,
		},
		Writer:       c,
		StatusClient: c,
	}

	return &tracingClient{Client: delegatingClient, scheme: options.Scheme}, nil
}

// helper functions
func setObjectTags(sp ot.Span, obj runtime.Object) {
	if gvk := obj.GetObjectKind().GroupVersionKind(); !gvk.Empty() {
		sp.SetTag("objectKind", gvk.String())
	}
	if m, err := meta.Accessor(obj); err == nil {
		sp.SetTag("objectKey", m.GetNamespace()+"/"+m.GetName())
	}
}

func traceError(sp ot.Span, err error) error {
	if err != nil {
		ext.Error.Set(sp, true)
		sp.LogFields(otlog.Error(err))
	}
	return err
}

// wrapper for Client which emits spans on each call
type tracingClient struct {
	client.Client
	scheme *runtime.Scheme
}

// go via scheme to find out what an object is
func (c *tracingClient) tagBlankObject(sp ot.Span, obj runtime.Object) {
	if c.scheme != nil {
		gvks, _, _ := c.scheme.ObjectKinds(obj)
		for _, gvk := range gvks {
			sp.SetTag("objectKind", gvk.String())
		}
	}
}

func (c *tracingClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.Get", ot.Tag{Key: "objectKey", Value: key.String()})
	defer sp.Finish()
	c.tagBlankObject(sp, obj)
	return traceError(sp, c.Client.Get(ctx, key, obj))
}

func (c *tracingClient) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.List")
	defer sp.Finish()
	c.tagBlankObject(sp, list)
	return traceError(sp, c.Client.List(ctx, list, opts...))
}

func (c *tracingClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.Create")
	defer sp.Finish()
	setObjectTags(sp, obj)
	return traceError(sp, c.Client.Create(ctx, obj, opts...))
}

func (c *tracingClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.Delete")
	defer sp.Finish()
	setObjectTags(sp, obj)
	return traceError(sp, c.Client.Delete(ctx, obj, opts...))
}

func (c *tracingClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.Update")
	defer sp.Finish()
	setObjectTags(sp, obj)
	return traceError(sp, c.Client.Update(ctx, obj, opts...))
}

func (c *tracingClient) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.Patch")
	defer sp.Finish()
	setObjectTags(sp, obj)
	return traceError(sp, c.Client.Patch(ctx, obj, patch, opts...))
}

func (c *tracingClient) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...client.DeleteAllOfOption) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.DeleteAllOf")
	defer sp.Finish()
	c.tagBlankObject(sp, obj)
	return traceError(sp, c.Client.DeleteAllOf(ctx, obj, opts...))
}

func (c *tracingClient) Status() client.StatusWriter {
	return &tracingStatusWriter{StatusWriter: c.Client.Status()}
}

type tracingStatusWriter struct {
	client.StatusWriter
}

func (s *tracingStatusWriter) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.Status.Update")
	defer sp.Finish()
	setObjectTags(sp, obj)
	return traceError(sp, s.StatusWriter.Update(ctx, obj, opts...))
}

func (s *tracingStatusWriter) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	sp, ctx := ot.StartSpanFromContext(ctx, "client.Status.Patch")
	defer sp.Finish()
	setObjectTags(sp, obj)
	return traceError(sp, s.StatusWriter.Patch(ctx, obj, patch, opts...))
}
