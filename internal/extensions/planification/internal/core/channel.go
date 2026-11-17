// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE MODULE CHANNEL — what a module receives and sends, read from the
// requests themselves: In, the open requests filed in it and from whom or
// what; Out, the requests it filed in other modules and where they stand; and
// the answers its questions received. The same verbs serve the GUI, agents,
// other modules and the outside (KTSP through the universe's hub).

// Channel answers a module's channel.
func (s *Service) Channel(ctx context.Context, in api.ChannelInput) (api.ChannelOutput, error) {
	if in.Module == "" {
		return api.ChannelOutput{}, invalid("Channel: name the module")
	}
	out := api.ChannelOutput{Module: in.Module, In: []api.ChannelItem{}, Out: []api.ChannelItem{}, Answers: []api.ChannelItem{}}
	received, err := s.List(ctx, api.ListInput{Module: in.Module})
	if err != nil {
		return api.ChannelOutput{}, err
	}
	for _, r := range received.Requests {
		if r.State != api.StateDone && r.State != api.StateDropped {
			out.In = append(out.In, channelItem(r))
		}
	}
	sent, err := s.List(ctx, api.ListInput{Requester: in.Module})
	if err != nil {
		return api.ChannelOutput{}, err
	}
	for _, r := range sent.Requests {
		if r.Requester.Kind != "module" || r.Module == in.Module {
			continue // its own follow-ups are In, not Out
		}
		item := channelItem(r)
		out.Out = append(out.Out, item)
		if r.Kind == api.KindQuestion && r.Answer != nil {
			out.Answers = append(out.Answers, item)
		}
	}
	for _, l := range [][]api.ChannelItem{out.In, out.Out, out.Answers} {
		sort.SliceStable(l, func(i, j int) bool { return l[i].FiledAt > l[j].FiledAt })
	}
	return out, nil
}

// channelItem reads a request into the channel: from whom or what.
func channelItem(r api.Request) api.ChannelItem {
	from := r.Requester.ID + " (" + r.Requester.Kind + ")"
	if src := r.Source; src != nil {
		from = src.Kind + " " + src.Name + " on " + src.Object
	}
	item := api.ChannelItem{ID: r.ID, Title: r.Title, Kind: r.Kind, State: r.State, Module: r.Module, From: from, FiledAt: r.FiledAt}
	if r.Answer != nil {
		item.Answer = r.Answer.Text
	}
	return item
}
