package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

/*
 * Trying once more, for the failures that are worth trying again.
 *
 * There was no retry anywhere. One dropped connection — a laptop changing
 * network, a hosted service shedding load for a second, Ollama still binding
 * its port after a restart — ended a conversation turn with an error, and the
 * person had to ask their question again. Work in the background survives it
 * because the orchestrator fails over to somebody else; a conversation has
 * nowhere to fail over to.
 *
 * Deliberately small. One more attempt, not five: a model call is expensive,
 * and a service that is genuinely down should be reported as down rather than
 * hammered until somebody notices. Nothing already streaming is ever retried —
 * half an answer has been delivered, and sending the question again would
 * write the rest of a different one.
 *
 * Only what cannot have been acted on: a request that never arrived, or one
 * the service refused with "too many" or "try later". A 400 is not retried,
 * because asking the same wrong question twice gets the same answer twice.
 */

const (
	// Tries is the first attempt and one more.
	Tries = 2

	// Waiting is how long to wait before the second attempt, unless the
	// service said how long itself.
	Waiting = 700 * time.Millisecond

	// LongestWait bounds what a service can ask us to wait. Beyond this it is
	// not a retry, it is a different conversation.
	LongestWait = 5 * time.Second
)

/*
 * sendWithOneMoreTry performs a request, building it afresh for each attempt.
 *
 * A factory rather than a request: a request body is read as it is sent, so
 * the same request cannot be sent twice.
 */
func sendWithOneMoreTry(ctx context.Context, client *http.Client, build func() (*http.Request, error)) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)

	for attempt := 1; attempt <= Tries; attempt++ {
		var req *http.Request

		req, err = build()
		if err != nil {
			return nil, err
		}

		resp, err = client.Do(req)

		if attempt == Tries || !worthAnotherTry(resp, err) {
			return resp, err
		}

		// The body of the attempt being abandoned, so the connection can be
		// used again rather than left dangling.
		wait := Waiting

		if resp != nil {
			if said := whenItSaidToTry(resp); said > 0 {
				wait = said
			}

			io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	return resp, err
}

// worthAnotherTry is the whole policy: what could succeed if asked again.
func worthAnotherTry(resp *http.Response, err error) bool {
	if err != nil {
		// Somebody stopped this, or the deadline passed. Asking again would
		// fail the same way and take longer about it.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}

		return true
	}

	if resp == nil {
		return false
	}

	switch resp.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// whenItSaidToTry reads Retry-After, in seconds or as a date, bounded.
func whenItSaidToTry(resp *http.Response) time.Duration {
	said := resp.Header.Get("Retry-After")
	if said == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(said); err == nil {
		return bounded(time.Duration(seconds) * time.Second)
	}

	if when, err := http.ParseTime(said); err == nil {
		return bounded(time.Until(when))
	}

	return 0
}

func bounded(wait time.Duration) time.Duration {
	switch {
	case wait <= 0:
		return 0
	case wait > LongestWait:
		return LongestWait
	default:
		return wait
	}
}
