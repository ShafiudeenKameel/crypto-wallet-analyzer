# Deploying

Backend on Render, frontend on Vercel - both free tier. They need each
other's URL, so there's a two-pass order: deploy the backend first, then
the frontend, then go back and tell the backend where the frontend ended
up.

## 1. Backend (Render)

1. [dashboard.render.com](https://dashboard.render.com) → **New** → **Blueprint** → connect this GitHub repo. Render reads [`render.yaml`](./render.yaml) and proposes one web service (`crypto-wallet-analyzer-backend`, rooted at `backend/`).
2. It'll prompt for the three env vars marked `sync: false`:
   - `ALLOWED_ORIGIN` - leave a placeholder for now (e.g. `http://localhost:5173`); you'll come back and fix this in step 4.
   - `ETHERSCAN_API_KEY`, `COINGECKO_API_KEY` - optional. Leave blank to run on each provider's public tier, or paste a free API key from [etherscan.io/apis](https://etherscan.io/apis) / [coingecko.com/api](https://www.coingecko.com/en/api) for higher rate limits.
3. Deploy. Note the resulting URL - `https://crypto-wallet-analyzer-backend.onrender.com` (Render names it after the service name, may differ slightly).
4. Sanity check once it's live: `curl https://<your-render-url>/healthz` should return `200`.

**Free tier note:** the service spins down after inactivity. The first request after a period of idleness will be slow (cold start) - expected, not a bug, and an accepted tradeoff for a free demo (see project.MD).

## 2. Frontend (Vercel)

1. [vercel.com/new](https://vercel.com/new) → import this repo.
2. Set **Root Directory** to `frontend` in the import screen (Vercel otherwise assumes the repo root). It auto-detects Vite from there via [`frontend/vercel.json`](./frontend/vercel.json).
3. Add an environment variable: `VITE_API_BASE_URL` = the Render URL from step 1.4 (no trailing slash).
4. Deploy. Note the resulting URL - `https://<your-project>.vercel.app`.

## 3. Close the loop

Go back to the Render dashboard → the backend service → Environment →
update `ALLOWED_ORIGIN` to the exact Vercel URL from step 2.4 (no
trailing slash). Render redeploys automatically on env var changes.

## 4. Verify

Open the Vercel URL, paste a real wallet address, pick a chain. You
should see a populated dashboard rather than the generic error message -
that error message is exactly what a CORS mismatch, a missing env var, or
Render still spinning up looks like, so if you hit it, check those three
things in order.
