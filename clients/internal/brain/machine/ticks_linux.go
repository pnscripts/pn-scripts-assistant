//go:build linux

package machine

/*
 * clockTicks is how many units of processor time make a second.
 *
 * The kernel reports process time in these rather than in seconds, and the
 * number is a compile-time constant of the kernel — 100 on every ordinary
 * build, which is why every tool that reads /proc assumes it. Kept as one
 * named thing so the assumption is visible rather than a 100 in an expression.
 */
func clockTicks() int { return 100 }
