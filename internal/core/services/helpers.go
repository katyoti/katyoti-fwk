// SPDX-License-Identifier: Apache-2.0

package services

import "context"

// UseService resolves a service by T's type name with instance "default".
func UseService[T any](sm *ServiceManager, ctx context.Context) (T, error) {
	return Get[T](sm, ctx, "default")
}

// UseNamedService resolves a service by T's type name with the given instance name.
func UseNamedService[T any](sm *ServiceManager, ctx context.Context, instanceName string) (T, error) {
	return Get[T](sm, ctx, instanceName)
}
