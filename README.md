# fly-request-echo

Minimal **lab-only** HTTP server: echoes **method, URL, query, every request header, and body** (up to 2 MiB) as JSON so you can see what **Fly.io’s edge** forwards (for example `Fly-Client-IP`, `Host`, `X-Forwarded-*`).

**Do not** point this at production secrets unless you set `ECHO_SECRET` (see below).

## Run locally

```bash
go run .
```

- JSON: `curl -s http://127.0.0.1:8080/ -X POST -d '{"hello":"world"}'`
- HTML: open `http://127.0.0.1:8080/?format=html` in a browser

## Deploy to Fly.io

1. Install the [Fly CLI](https://fly.io/docs/hands-on/install-flyctl/) and log in: `fly auth login`.

2. From this directory, create a **new** app name (globally unique):

   ```bash
   fly apps create your-echo-test-unique
   ```

3. Edit `fly.toml` and set `app = "your-echo-test-unique"` to match.

4. Deploy:

   ```bash
   fly deploy
   ```

5. Optional: require a shared secret on the echo route ( **`/health` stays public** for Fly checks):

   ```bash
   fly secrets set ECHO_SECRET="$(openssl rand -hex 16)"
   ```

   Then:

   ```bash
   curl -s https://your-app.fly.dev/ -H "X-Echo-Secret: <value>" -X POST -d test=1
   ```

## Optional custom hostname

Add a certificate for `echo.example.com` in the Fly dashboard (or `fly certs add echo.example.com`) and DNS `CNAME` to your app’s `*.fly.dev` target; the JSON `host` field will show which hostname the client used.
