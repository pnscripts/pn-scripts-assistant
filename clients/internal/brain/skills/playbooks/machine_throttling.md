name: machine_throttling
version: 1
when: I ask to run several builds, builders, sub-agents or heavy jobs at once, the machine is slow or overloaded, the load average is high, or before starting a test suite, Blender, Godot, headless Chrome, phpunit or go build
tools: run_command, list_background, do_in_background, read_file
---

The workstation has four CPU cores. On 2026-10-10 many builders running beside the pnscripts.com test suite drove the load average to about 33, everything crawled and the builders stalled each other. The lesson is DEV/ai-brain/.ai/memory/lessons/20261010-throttle-factory-builders-on-4-cores.md (DEV is the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV).

The limits:
- At most two or three builders at once.
- One heavy job at a time across everything: phpunit or php artisan test, Blender, Godot, headless Chrome or node tests, go build.
- The pnscripts.com full suite (about 80 to 90 minutes) runs alone, never beside builders.
- Heavy commands run with nice -n 19.
- No busy-wait loops. When waiting, wait at least 30 seconds between checks, or rely on a background job's completion.
- No dev servers left running, and no parallel deep-search solvers.

## Before you start

1. Read the load: run_command ["uptime"]. The three numbers are the 1, 5 and 15-minute load averages. Below 4 the machine has room; between 4 and 8 start nothing heavy; above 8 wait.
2. See what is running: run_command ["bash","-lc","ps -eo pid,ni,pcpu,pmem,etime,comm --sort=-pcpu | head -15"]. Look for php, blender, godot, chrome, node, go and python processes.
3. Check your own background work with list_background.
4. Free memory: run_command ["free","-h"].

## Steps

1. Count the heavy jobs already running (from ps). If one is running, queue yours instead of starting it.
2. Start a heavy job with nice: ["nice","-n","19", ...]. For jobs longer than five minutes use do_in_background, or ["bash","-lc","setsid nohup nice -n 19 <command> > <log> 2>&1 & echo $!"] and keep the printed process id.
3. Poll a long job by reading its log or by checking that its process id is alive (ps -p <pid>), with at least 30 seconds, preferably several minutes, between checks.
4. Run builders one after another within one session. When several sessions build at once, keep the total at two or three.
5. Stop a process by its recorded process id (kill <pid>). Do not use pkill -f or pgrep -f with a pattern: the pattern matches the command line of the shell running it, and the shell kills itself.
6. When a job finishes, check that no stray Chrome or server processes from it remain, and stop them by process id.

## Verify

- uptime before and after, quoted in the report.
- The list of running heavy processes was checked before starting, and no more than one heavy job ran at a time.
- Long jobs left a log, and the report quotes its last lines.

## Never

- Never write a loop that checks something without sleeping, and never poll faster than every 30 seconds.
- Never start the full site suite beside builders, and never start more than three builders.
- Never use pkill -f or killall on broad patterns; never kill processes you did not start without asking the owner.
- Never leave dev servers running after a check.
- Never change system settings, CPU governors or swap without the owner's yes.
- Never type passwords; if sudo is needed, ask the owner to run the command himself.
