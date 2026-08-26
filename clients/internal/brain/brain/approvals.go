package brain

import (
	"context"
	"encoding/json"
	"fmt"

	"pn-brain/internal/brain/store"
)

// PendingApprovals lists actions waiting for a decision.
func (b *Brain) PendingApprovals() ([]store.Invocation, error) {
	return b.DB.PendingInvocations()
}

// Decide records the owner's answer and, if allowed, carries the action out.
//
// The order here is deliberate and is the whole point of the gate. The decision
// is written first, and only a transition that actually moved the row from
// pending is acted on. An action already decided cannot be decided again, so a
// repeated or replayed request cannot turn a denial into an approval or run
// something twice.
func (b *Brain) Decide(ctx context.Context, id int64, approve bool) (store.Invocation, error) {
	invocation, err := b.DB.Invocation(id)
	if err != nil {
		return store.Invocation{}, err
	}

	if invocation == nil {
		return store.Invocation{}, fmt.Errorf("there is no action with id %d", id)
	}

	decision := store.InvocationDenied
	if approve {
		decision = store.InvocationApproved
	}

	moved, err := b.DB.DecideInvocation(id, decision)
	if err != nil {
		return store.Invocation{}, err
	}

	if !moved {
		return *invocation, fmt.Errorf(
			"that action was already %s and cannot be decided again", invocation.Status)
	}

	if !approve {
		invocation.Status = store.InvocationDenied

		return *invocation, nil
	}

	tool, ok := b.Agent.Registry.Get(invocation.Tool)
	if !ok {
		b.DB.CompleteInvocation(id, store.InvocationFailed, "the tool no longer exists")
		invocation.Status = store.InvocationFailed

		return *invocation, fmt.Errorf("the %q tool no longer exists", invocation.Tool)
	}

	// The stored arguments are used, never anything supplied with the approval.
	// Otherwise the thing approved and the thing performed could differ, and the
	// summary the owner read would be a description of something that did not
	// happen.
	output, execErr := tool.Execute(ctx, json.RawMessage(invocation.Arguments))

	status := store.InvocationDone
	result := output

	if execErr != nil {
		status = store.InvocationFailed
		result = execErr.Error()
	}

	if err := b.DB.CompleteInvocation(id, status, result); err != nil {
		return *invocation, err
	}

	invocation.Status = status
	invocation.Result = result

	b.Log.Info("carried out an approved action", "id", id, "tool", invocation.Tool, "status", status)

	return *invocation, execErr
}
