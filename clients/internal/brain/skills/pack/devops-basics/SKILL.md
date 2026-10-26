---
name: devops-basics
description: Help set up and run the delivery side of a software project - environments, configuration and secrets, Docker containers, CI/CD pipelines (GitHub Actions, GitLab CI), deployments with rollback, a basic Linux server, HTTPS, backups, logs and monitoring. Use when the user asks how to deploy an app, write a Dockerfile or pipeline, why a build passes locally but fails in CI, how to store secrets, set up a server or domain with HTTPS, schedule backups, or find out why production is down.
---

# DevOps basics

Make every release repeatable, reversible and observable: the same build goes through test and production, secrets stay out of code, and you can see and undo what happened.

## Steps
1. **Map the current setup.** What runs where (laptop, shared host, VPS, cloud service), how code gets there today, database, domain and DNS, who has access, and what breaks most often.
2. **Separate environments.** Development, staging (production-like, fake or anonymised data) and production. Configuration comes from environment variables, not edited files on the server.
3. **Handle secrets.** Keep API keys and passwords in the platform's secret store or a `.env` file that is git-ignored; commit a `.env.example` with names only. If a secret was ever committed, rotate it, do not only delete the line.
4. **Containerise when it helps.** One process per container, small base image, pinned versions, non-root user.
   ```dockerfile
   FROM node:22-slim
   WORKDIR /app
   COPY package*.json ./
   RUN npm ci --omit=dev
   COPY . .
   USER node
   EXPOSE 3000
   CMD ["node", "server.js"]
   ```
5. **Automate with CI/CD.** On every push: install, lint, test, build. On the main branch or a tag: build the artefact once, deploy to staging, smoke-test, then promote to production.
   ```yaml
   name: ci
   on: [push, pull_request]
   jobs:
     test:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         - uses: actions/setup-node@v4
           with: { node-version: 22, cache: npm }
         - run: npm ci
         - run: npm test
   ```
   Check the current major versions of actions in their official docs.
6. **Deploy safely.** Database migrations backward-compatible with the old code, health check endpoint, keep the previous release to roll back in one command, deploy at a time someone can watch.
7. **Harden a basic server.** SSH keys only, no root login, firewall with only needed ports (22, 80, 443), automatic security updates, HTTPS with a free certificate that renews itself, reverse proxy in front of the app.
8. **Back up and test restores.** Database and uploaded files daily, kept off the server, several generations (for example 7 daily, 4 weekly); restore to a test machine at least monthly.
9. **Observe.** Central logs with timestamps, uptime check from outside, alerts for errors, disk full and certificate expiry.

## Incident checklist ("production is down")
1. Confirm from outside (another network, uptime monitor) and note the time.
2. What changed last? Deploy, config, certificate, DNS, disk, provider status page.
3. Read logs around the first error; check disk space, memory and the process status.
4. Roll back the last change if it is the likely cause; fix forward later.
5. Write a short note: cause, fix, how to prevent it.

## Pitfalls
- "Works on my machine": different runtime versions, missing env variables, case-sensitive paths on Linux, files not committed. Pin versions and build in CI from a clean checkout.
- Using the `latest` tag for images makes builds unrepeatable.
- Backups that were never restored are hopes, not backups.
- Pipelines that print environment variables can leak secrets in logs.
- Manual hotfixes on the server get overwritten by the next deploy; fix in the repository.

## Output
Hand back the files or commands needed (Dockerfile, pipeline, server steps) adapted to the user's stack, the order to apply them, how to verify each step, and the rollback path.
