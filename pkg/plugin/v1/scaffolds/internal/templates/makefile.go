/*
Copyright 2019 The Kubernetes Authors.
Modifications copyright 2020 The Operator-SDK Authors

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

package templates

import (
	"sigs.k8s.io/kubebuilder/pkg/model/file"
)

var _ file.Template = &Makefile{}

// Makefile scaffolds the Makefile
type Makefile struct {
	file.TemplateMixin

	// Image is controller manager image name
	Image string
	//Kustomize version to use in the project
	KustomizeVersion string
}

// SetTemplateDefaults implements input.Template
func (f *Makefile) SetTemplateDefaults() error {
	if f.Path == "" {
		f.Path = "Makefile"
	}

	f.TemplateBody = makefileTemplate

	f.IfExistsAction = file.Error

	if f.Image == "" {
		f.Image = "controller:latest"
	}

	return nil
}

//nolint:lll
const makefileTemplate = `
# Image URL to use all building/pushing image targets
IMG ?= {{ .Image }}

all: manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run: helm-operator
	$(HELM_OPERATOR) run

# Install CRDs into a cluster
install: kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Undeploy controller in the configured Kubernetes cluster in ~/.kube/config
undeploy: kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl delete -f -

# Build the docker image
docker-build:
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

PATH  := $(PATH):$(PWD)/bin
SHELL := env PATH=$(PATH) /bin/sh
OS    = $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH  = $(shell uname -m | sed 's/x86_64/amd64/')

kustomize:
ifeq (, $(shell which kustomize 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p bin ;\
	curl -sSLo - https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/{{ .KustomizeVersion }}/kustomize_{{ .KustomizeVersion }}_${OS}_${ARCH}.tar.gz | tar xzf - -C bin/ ;\
	}
KUSTOMIZE=./bin/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

# TODO(joelanford): need to download helm-operator binary for user
#    Can't rely on Go to download and build
helm-operator:
ifeq (, $(shell which helm-operator 2>/dev/null))
	@{ \
	echo "ERROR: You must download helm-operator binary and add it to your path!" ;\
	exit 1 ;\
	}
else
HELM_OPERATOR=$(shell which helm-operator)
endif
`
