package main

const fileTpl = `// Copyright 2022 The PipeCD Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by protoc-gen-auth. DO NOT EDIT.
// source: {{ .InputPath }}

package webservice

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/pipe-cd/pipecd/pkg/cache"
	"github.com/pipe-cd/pipecd/pkg/cache/memorycache"
	"github.com/pipe-cd/pipecd/pkg/config"
	"github.com/pipe-cd/pipecd/pkg/datastore"
	"github.com/pipe-cd/pipecd/pkg/model"
	"github.com/pipe-cd/pipecd/pkg/rpc/rpcauth"
)

type webApiProjectStore interface {
	Get(ctx context.Context, id string) (*model.Project, error)
}

type authorizer struct {
	projectStore webApiProjectStore
	rbacCache    cache.Cache
	// List of debugging/quickstart projects.
	projectsInConfig map[string]config.ControlPlaneProject
	logger           *zap.Logger
}

// NewRBACAuthorizer returns an RBACAuthorizer object for checking requested method based on RBAC.
func NewRBACAuthorizer(
	ctx context.Context,
	ds datastore.DataStore,
	projects map[string]config.ControlPlaneProject,
	logger *zap.Logger,
) rpcauth.RBACAuthorizer {
	w := datastore.WebCommander
	return &authorizer{
		projectStore:     datastore.NewProjectStore(ds, w),
		rbacCache:        memorycache.NewTTLCache(ctx, 10*time.Minute, 5*time.Minute),
		projectsInConfig: projects,
		logger:           logger.Named("authorizer"),
	}
}

func (a *authorizer) getRBAC(ctx context.Context, projectID string) (*rbac, error) {
	if _, ok := a.projectsInConfig[projectID]; ok {
		p := &model.Project{Id: projectID}
		p.SetBuiltinRBACRoles()
		return &rbac{p.RbacRoles}, nil
	}

	r, err := a.rbacCache.Get(projectID)
	if err == nil {
		return r.(*rbac), nil
	}
	a.logger.Debug("unable to get the rbac cache in memory cache", zap.Error(err))

	p, err := a.projectStore.Get(ctx, projectID)
	if err != nil {
		a.logger.Error("failed to get project",
			zap.String("project", projectID),
			zap.Error(err),
		)
		return nil, err
	}

	v := &rbac{p.RbacRoles}
	if err = a.rbacCache.Put(projectID, v); err != nil {
		a.logger.Warn("unable to store the rbac in memory cache",
			zap.String("project", projectID),
			zap.Error(err),
		)
	}
	return v, nil
}

type rbac struct {
	Roles []*model.ProjectRBACRole
}

func (r *rbac) HasPermission(typ model.ProjectRBACResource_ResourceType, action model.ProjectRBACPolicy_Action) bool {
	for _, v := range r.Roles {
		if v.HasPermission(typ, action) {
			return true
		}
	}
	return false
}

func (r *rbac) FilterByNames(names []string) *rbac {
	roles := make([]*model.ProjectRBACRole, 0, len(names))
	rs := make(map[string]*model.ProjectRBACRole, len(r.Roles))
	for _, v := range r.Roles {
		rs[v.Name] = v
	}
	for _, n := range names {
		if v, ok := rs[n]; ok {
			roles = append(roles, v)
		}
	}
	r.Roles = roles
	return r
}

// Authorize checks whether a role is enough for given gRPC method or not.
func (a *authorizer) Authorize(ctx context.Context, method string, r model.Role) bool {
	rbac, err := a.getRBAC(ctx, r.ProjectId)
	if err != nil {
		return false
	}
	rbac.FilterByNames(r.ProjectRbacRoles)

	switch method {
	{{- range .Methods }}
	case "/grpc.service.webservice.WebService/{{ .Name }}":
	        {{- if .Ignored }}
			return true
		{{- else }}
			return rbac.HasPermission(model.ProjectRBACResource_{{ .Resource }}, model.ProjectRBACPolicy_{{ .Action }})
		{{- end }}
	{{- end }}
	}
	return false
}
`
