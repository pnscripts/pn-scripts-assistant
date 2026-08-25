// One module for every Pnexus client so they can share the preflight logic
// instead of each carrying a copy. The desktop app needs to know what the
// machine is missing before the brain exists, which is exactly what the doctor
// knows — duplicating that would guarantee the two drift apart.
module pnexus

go 1.25.0
