// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// The Store of a SchedulerStore: each call goes to the inner store chosen at
// the first one (chosen); a choice that failed answers its error.

func (s *SchedulerStore) Get(ctx context.Context, id string) (api.Todo, error) {
	st, err := s.chosen()
	if err != nil {
		return api.Todo{}, err
	}
	return st.Get(ctx, id)
}

func (s *SchedulerStore) List(ctx context.Context, module string, done bool) ([]api.Todo, error) {
	st, err := s.chosen()
	if err != nil {
		return nil, err
	}
	return st.List(ctx, module, done)
}

func (s *SchedulerStore) Create(ctx context.Context, t api.Todo) (api.Todo, error) {
	st, err := s.chosen()
	if err != nil {
		return api.Todo{}, err
	}
	return st.Create(ctx, t)
}

func (s *SchedulerStore) Put(ctx context.Context, t api.Todo) (api.Todo, error) {
	st, err := s.chosen()
	if err != nil {
		return api.Todo{}, err
	}
	return st.Put(ctx, t)
}

func (s *SchedulerStore) Update(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	st, err := s.chosen()
	if err != nil {
		return api.Todo{}, err
	}
	return st.Update(ctx, id, fn)
}

func (s *SchedulerStore) Claim(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	st, err := s.chosen()
	if err != nil {
		return api.Todo{}, err
	}
	return st.Claim(ctx, id, fn)
}

func (s *SchedulerStore) Archive(ctx context.Context, id string, fn func(t *api.Todo) error) (api.Todo, error) {
	st, err := s.chosen()
	if err != nil {
		return api.Todo{}, err
	}
	return st.Archive(ctx, id, fn)
}

func (s *SchedulerStore) RemoveDone(ctx context.Context, id string) error {
	st, err := s.chosen()
	if err != nil {
		return err
	}
	return st.RemoveDone(ctx, id)
}

func (s *SchedulerStore) Revisions(ctx context.Context, id string) ([]api.Todo, error) {
	st, err := s.chosen()
	if err != nil {
		return nil, err
	}
	return st.Revisions(ctx, id)
}

func (s *SchedulerStore) AddWait(ctx context.Context, module, on string, mk func(id string) api.Wait) (api.Wait, error) {
	st, err := s.chosen()
	if err != nil {
		return api.Wait{}, err
	}
	return st.AddWait(ctx, module, on, mk)
}

func (s *SchedulerStore) ListWaits(ctx context.Context, module string) ([]api.Wait, error) {
	st, err := s.chosen()
	if err != nil {
		return nil, err
	}
	return st.ListWaits(ctx, module)
}

func (s *SchedulerStore) PutWait(ctx context.Context, w api.Wait) error {
	st, err := s.chosen()
	if err != nil {
		return err
	}
	return st.PutWait(ctx, w)
}

func (s *SchedulerStore) RemoveWait(ctx context.Context, id string) error {
	st, err := s.chosen()
	if err != nil {
		return err
	}
	return st.RemoveWait(ctx, id)
}

func (s *SchedulerStore) Atomically(ctx context.Context, fn func(ctx context.Context) error) error {
	st, err := s.chosen()
	if err != nil {
		return err
	}
	return st.Atomically(ctx, fn)
}
