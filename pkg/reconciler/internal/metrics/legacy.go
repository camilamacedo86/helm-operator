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
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type LegacyInfoGauge struct {
	*prometheus.GaugeVec
}

func legacyInfoGaugeName(kind string) string {
	return fmt.Sprintf("%s_info", strings.ToLower(kind))
}

func NewLegacyInfoGauge(kind string) *LegacyInfoGauge {
	return &LegacyInfoGauge{
		prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: legacyInfoGaugeName(kind),
			Help: fmt.Sprintf("Information about the %s custom resource", kind),
		}, []string{"namespace", "name"}),
	}
}

func (vec *LegacyInfoGauge) Create(e event.CreateEvent) {
	name := e.Meta.GetName()
	namespace := e.Meta.GetNamespace()
	vec.set(name, namespace)
}

func (vec *LegacyInfoGauge) Update(e event.UpdateEvent) {
	name := e.MetaNew.GetName()
	namespace := e.MetaNew.GetNamespace()
	vec.set(name, namespace)
}

func (vec *LegacyInfoGauge) Delete(e event.DeleteEvent) {
	vec.GaugeVec.Delete(map[string]string{
		"name":      e.Meta.GetName(),
		"namespace": e.Meta.GetNamespace(),
	})
}

func (vec *LegacyInfoGauge) set(name, namespace string) {
	labels := map[string]string{
		"name":      name,
		"namespace": namespace,
	}
	m, err := vec.GaugeVec.GetMetricWith(labels)
	if err != nil {
		panic(err)
	}
	m.Set(1)
}

func NewLegacyRegistry(kind string) RegistererGathererPredicater {
	crInfo := NewLegacyInfoGauge(kind)
	r := NewRegistry()
	r.MustRegister(crInfo)
	return r
}
