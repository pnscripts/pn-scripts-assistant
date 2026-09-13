package brain

import (
	"context"
	"encoding/json"
	"fmt"

	"pn-scripts-assistant/internal/brain/protect"
	"pn-scripts-assistant/internal/brain/store"
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

		b.carryOn(ctx, *invocation)

		return *invocation, nil
	}

	tool, ok := b.Agent.Registry.Get(invocation.Tool)
	if !ok {
		b.DB.CompleteInvocation(id, store.InvocationFailed, "the tool no longer exists")
		invocation.Status = store.InvocationFailed

		return *invocation, fmt.Errorf("the %q tool no longer exists", invocation.Tool)
	}

	/*
	 * And it learns the answer, so it does not ask twice about the same file.
	 *
	 * This is the whole point of asking rather than refusing. A question put a
	 * second time about a thing already answered is a question that stops
	 * being read and starts being clicked through, and at that moment the
	 * prompt has become worse than useless — it looks like protection and is
	 * not.
	 *
	 * One file, not the rule it matched: saying yes to one .env in one project
	 * must not quietly open every .env on the machine. The list of what it has
	 * learned is shown in the privacy panel and any of it can be taken back.
	 */
	if path, _, held := protect.InArguments(json.RawMessage(invocation.Arguments)); held {
		protect.Learn(b.Root, path)

		b.Log.Info("learned that a protected file is allowed",
			"path", path, "tool", invocation.Tool)
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

	b.carryOn(ctx, *invocation)

	return *invocation, execErr
}

/*
 * carryOn lets whatever was waiting on this decision get on with it.
 *
 * Both routes into Decide come through here — the button in the interface and
 * the decide_waiting tool somebody uses out loud — so saying "approve
 * everything waiting" resumes parked tasks with no extra machinery.
 *
 * A conversation needs none of this: it ended, and its owner asks again. A
 * task has work behind it and nobody to ask, which is the whole difference.
 */
func (b *Brain) carryOn(ctx context.Context, invocation store.Invocation) {
	if b.Tasks == nil {
		return
	}

	if err := b.Tasks.Resolved(ctx, invocation); err != nil {
		b.Log.Warn("could not carry on with what was waiting on this",
			"invocation", invocation.ID, "error", err)
	}
}
