/*
Copyright 2020 The Operator-SDK Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"context"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var (
	log = logf.Log.WithName("kube-state-metrics")
)

type CreateEventHandler interface {
	Create(e event.CreateEvent)
}

type UpdateEventHandler interface {
	Update(e event.UpdateEvent)
}

type DeleteEventHandler interface {
	Delete(e event.DeleteEvent)
}

type GenericEventHandler interface {
	Generic(e event.GenericEvent)
}

type RegistererGathererPredicater interface {
	prometheus.Registerer
	prometheus.Gatherer
	Predicate() predicate.Predicate
}

type Registry struct {
	*prometheus.Registry
	metrics []prometheus.Collector
}

func NewRegistry() RegistererGathererPredicater {
	return &Registry{
		Registry: prometheus.NewRegistry(),
	}
}

func (r *Registry) Register(c prometheus.Collector) error {
	if err := r.Registry.Register(c); err != nil {
		return err
	}
	r.metrics = append(r.metrics, c)
	return nil
}

func (r *Registry) MustRegister(cs ...prometheus.Collector) {
	for _, c := range cs {
		if err := r.Register(c); err != nil {
			panic(err)
		}
	}
}

func (r *Registry) Predicate() predicate.Predicate {
	createHandlers := []CreateEventHandler{}
	updateHandlers := []UpdateEventHandler{}
	deleteHandlers := []DeleteEventHandler{}
	genericHandlers := []GenericEventHandler{}

	for _, m := range r.metrics {
		if m, ok := m.(CreateEventHandler); ok {
			createHandlers = append(createHandlers, m)
		}
		if m, ok := m.(UpdateEventHandler); ok {
			updateHandlers = append(updateHandlers, m)
		}
		if m, ok := m.(DeleteEventHandler); ok {
			deleteHandlers = append(deleteHandlers, m)
		}
		if m, ok := m.(GenericEventHandler); ok {
			genericHandlers = append(genericHandlers, m)
		}
	}

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			for _, m := range createHandlers {
				m.Create(e)
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			for _, m := range updateHandlers {
				m.Update(e)
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			for _, m := range deleteHandlers {
				m.Delete(e)
			}
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			for _, m := range genericHandlers {
				m.Generic(e)
			}
			return true
		},
	}
}

type Server struct {
	Gatherer      prometheus.Gatherer
	ListenAddress string
}

const metricsPath = "/metrics"

func (s *Server) Start(stop <-chan struct{}) error {
	log.Info("metrics server is starting to listen", "addr", s.ListenAddress)
	l, err := net.Listen("tcp", s.ListenAddress)
	if err != nil {
		return err
	}

	handler := promhttp.HandlerFor(s.Gatherer, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	mux := http.NewServeMux()
	mux.Handle(metricsPath, handler)

	server := http.Server{
		Handler: mux,
	}

	errChan := make(chan error)
	go func() {
		log.Info("starting metrics server", "path", metricsPath)
		if err := server.Serve(l); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		if err := server.Shutdown(context.Background()); err != nil {
			return err
		}
	}
	return nil
}
