# macOS bootstrap

This file describes how to create the repository before asking Codex to implement application code.

## 1. Check the machine

Open Terminal and run:

```bash
uname -m
git --version
go version
node --version
npm --version
docker --version
codex --version
```

Do not worry if Node or Docker is not installed yet; the backend bootstrap only requires Git and Go. Docker will be used for local PostgreSQL. Node is needed when the client application is created.

The project targets Go 1.27 or newer compatible 1.x versions.

## 2. Install missing tools

If Homebrew is already installed:

```bash
brew --version
```

Install Go and Git if needed:

```bash
brew install go git
```

Install Node when you are ready to work on the frontend:

```bash
brew install node
```

Install Docker Desktop if you do not already have a Docker-compatible local runtime. After installation, verify:

```bash
docker version
docker compose version
```

Install or update Codex CLI with npm:

```bash
npm install -g @openai/codex
codex --version
```

## 3. Create the repository

Choose a workspace directory, for example:

```bash
mkdir -p ~/Projects
cd ~/Projects
mkdir rc-setup-hub
cd rc-setup-hub
git init
```

Copy the starter documentation into this repository so that the root contains `AGENTS.md`, `README.md`, `BOOTSTRAP_MAC.md`, and `AI/`.

Then create the implementation directories:

```bash
mkdir -p backend frontend
```

`frontend/` can remain empty until the client technology is finalized.

## 4. Initialize the Go backend

```bash
cd backend
go mod init github.com/YOUR_GITHUB_USERNAME/rc-setup-hub/backend
go mod tidy
cd ..
```

Replace `YOUR_GITHUB_USERNAME` before running the command. If the final Git remote will use another host or organization, use that repository path instead.

Check:

```bash
cat backend/go.mod
go version
go env GOMOD
```

When run from `backend/`, `go env GOMOD` should point to `backend/go.mod`.

## 5. Create the initial backend directories

```bash
mkdir -p backend/cmd/server
mkdir -p backend/internal/config
mkdir -p backend/internal/database
mkdir -p backend/internal/auth
mkdir -p backend/internal/users
mkdir -p backend/internal/friendships
mkdir -p backend/internal/chassis
mkdir -p backend/internal/setups
mkdir -p backend/internal/admin
mkdir -p backend/internal/web
mkdir -p backend/migrations
mkdir -p backend/templates/admin
```

Do not generate application code yet.

## 6. Add the backend-specific Codex instructions

Create `backend/AGENTS.md` with the contents from the starter bundle.

Because this file is closer to code than the root `AGENTS.md`, it can contain backend-specific constraints while the root file stays short.

## 7. Verify Codex sees the repository

From the repository root:

```bash
codex
```

In Codex, ask:

```text
Read AGENTS.md and AI/main.md. Do not change any files. Summarize the POC scope, database approach, and the first roadmap milestone.
```

The answer should mention:

- Go backend + PostgreSQL;
- separate client over CORS;
- server-rendered admin panel in the backend;
- admin emails from environment;
- `setups.data` stored as structured JSONB;
- nullable `chassis_model_id`;
- required `schema_version`;
- first milestone is bootstrap/health/database connectivity.

If any of those are missing, do not start implementation yet. Fix the documentation/context first.

## 8. Make the first commit

```bash
git add .
git status
git commit -m "Initialize RC Setup Hub project documentation"
```

At this point stop. The next step is to confirm that the repository was created successfully before implementation starts.
