# PulsePoll — Real-Time Polling Platform

PulsePoll is a portfolio-ready live polling application built for the GUVI Developer Internship task. It follows the required architecture: **React frontend + Go/Gin backend + MongoDB + Redis**, with Redis Pub/Sub actually driving WebSocket updates. The task brief requires end-to-end use, backend validation, real-time behavior, basic authenticated poll management, public deployment, a public GitHub repository, and a 3–5 minute demo video.

## Features

- JWT signup/login for poll creators
- Authenticated poll creation and management
- Public shareable poll URLs
- 2–10 validated poll options
- Optional poll expiry
- One vote per browser identity per poll, enforced by a MongoDB unique index
- Voter key is SHA-256 hashed before storage
- MongoDB stores users, polls, and durable votes
- Redis stores live vote counters
- Redis Pub/Sub publishes vote/status events
- Redis subscriber broadcasts those events to WebSocket clients
- Automatic WebSocket reconnect in the frontend
- Vote rate limiting through Redis
- Poll dashboard with open/close/delete controls
- Responsive UI
- Health endpoint for deployment monitoring

## Architecture

```text
React / Browser
      │
      ├── REST: auth, polls, votes ───────────────┐
      │                                           ▼
      │                                      Go + Gin
      │                                           │
      │                          ┌────────────────┴───────────────┐
      │                          ▼                                ▼
      │                      MongoDB                           Redis
      │                 durable source                 live counters + Pub/Sub
      │                                                           │
      │                                                           ▼
      └──────────── WebSocket ◀──────── Redis subscriber ◀────────┘
```

**Important realtime path:** a successful vote is persisted to MongoDB, the corresponding Redis counter is incremented, and a Redis Pub/Sub event is published. A backend Redis subscriber receives that event and sends it to every WebSocket client watching that poll. The API does not directly broadcast the event, so Redis is part of the actual realtime path.

## Project structure

```text
pulsepoll/
├── frontend/
│   ├── src/
│   │   ├── main.jsx
│   │   └── styles.css
│   ├── index.html
│   ├── package.json
│   └── vercel.json
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── auth/
│   │   ├── handlers/
│   │   ├── models/
│   │   ├── realtime/
│   │   ├── repository/
│   │   └── routes/
│   ├── Dockerfile
│   ├── .env.example
│   └── go.mod
├── docker-compose.yml
├── render.yaml
└── README.md
```

## Run locally

### Option A — Docker Compose

From the project root:

```bash
docker compose up --build
```

Open:

- Frontend: `http://localhost:5173`
- Backend health: `http://localhost:8080/health`

### Option B — run services separately

Start MongoDB and Redis, then:

```bash
cd backend
cp .env.example .env
go mod tidy
go run ./cmd/server
```

In another terminal:

```bash
cd frontend
npm install
npm run dev
```

## API endpoints

| Method | Endpoint | Auth | Purpose |
|---|---|---|---|
| POST | `/api/auth/signup` | No | Create creator account |
| POST | `/api/auth/login` | No | Login |
| POST | `/api/polls` | Yes | Create poll |
| GET | `/api/polls` | Yes | List creator polls |
| GET | `/api/polls/:id` | No | View poll + live counts |
| POST | `/api/polls/:id/vote` | No | Cast one vote |
| POST | `/api/polls/:id/close` | Yes | Close owned poll |
| DELETE | `/api/polls/:id` | Yes | Delete owned poll |
| GET | `/api/polls/:id/ws` | No | Live WebSocket stream |
| GET | `/health` | No | Health check |

## Deployment plan

The repository includes a Render backend configuration and a Vercel SPA rewrite. For a public deployment, create:

1. A MongoDB Atlas database and obtain its connection string.
2. A Redis deployment and obtain its address/URL in the format required by the backend.
3. A Render web service from `backend/`, setting `MONGO_URI`, `MONGO_DB`, `REDIS_ADDR`, `JWT_SECRET`, and `FRONTEND_ORIGIN`.
4. A Vercel project from `frontend/`, setting `VITE_API_URL` to the deployed backend's `/api` URL.
5. Set `FRONTEND_ORIGIN` on the backend to the exact deployed frontend origin.
6. Test signup → create poll → copy/open poll in two browser windows → vote → verify both windows update without refresh.

The task explicitly requires the final solution to be publicly deployed and to include the live link in the submission.

## Demo checklist

For the required 3–5 minute video, demonstrate:

1. Sign up/login.
2. Create a poll.
3. Open the public poll in two windows.
4. Vote in one window and show the other window update without refresh.
5. Open the dashboard and close the poll.
6. Explain the hardest challenge: making Redis genuinely drive live WebSocket updates rather than using Redis only as a decorative cache.
7. State which AI tools were used and what was reviewed/understood manually, as required by the brief.

## Security notes

- Passwords are bcrypt-hashed.
- JWTs are signed with a server-side secret.
- Request bodies are size-limited.
- Poll/question/option input is validated on the backend.
- Poll management requires ownership through JWT identity.
- Duplicate votes are protected by a MongoDB unique compound index.
- Raw voter keys are not stored; only SHA-256 hashes are persisted.
- Vote requests are rate-limited with Redis.
- CORS is restricted to the configured frontend origin.

## License

This project is an internship/portfolio implementation. Add the dataset/source license only if external datasets are introduced later.
