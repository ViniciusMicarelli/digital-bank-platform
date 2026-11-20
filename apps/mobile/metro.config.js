// The app imports the shared TypeScript in packages/ts-core (alias
// @bank/core in tsconfig.json), which lives outside the project root.
const { getDefaultConfig } = require('expo/metro-config')
const http = require('http')
const path = require('path')

const config = getDefaultConfig(__dirname)
config.watchFolders = [path.resolve(__dirname, '../../packages/ts-core')]
config.resolver.nodeModulesPaths = [path.resolve(__dirname, 'node_modules')]

// In development the dev server also forwards the API (/v1/*) to the
// gateway, so a phone that can load the app can always reach the API through
// the same address (LAN, tunnel, or a computer on two networks), and the web
// build needs no CORS. Streams (Server-Sent Events) are piped as they arrive.
const target = new URL(process.env.API_PROXY_TARGET || 'http://127.0.0.1:8000')
config.server = config.server || {}
const enhance = config.server.enhanceMiddleware
config.server.enhanceMiddleware = (middleware, server) => {
  const next = enhance ? enhance(middleware, server) : middleware
  return (req, res, fallthrough) => {
    if (!req.url.startsWith('/v1/') && req.url !== '/healthz') return next(req, res, fallthrough)
    const headers = { ...req.headers, host: target.host }
    delete headers.origin // same-origin from the app's point of view
    // Like any reverse proxy: append the phone's address for the gateway.
    const peer = (req.socket.remoteAddress || '').replace(/^::ffff:/, '')
    headers['x-forwarded-for'] = headers['x-forwarded-for'] ? `${headers['x-forwarded-for']}, ${peer}` : peer
    const upstream = http.request({ hostname: target.hostname, port: target.port, path: req.url, method: req.method, headers }, (r) => {
      res.writeHead(r.statusCode || 502, r.headers)
      r.pipe(res)
    })
    upstream.on("error", (err) => {
      if (res.headersSent) return res.end()
      res.writeHead(502, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ error: { code: 'GATEWAY_DOWN', message: `API unreachable at ${target.origin} (${err.code || err.message}): run make up` } }))
    })
    // The client went away (e.g. a closed stream): stop the upstream request.
    res.on('close', () => { if (!res.writableEnded) upstream.destroy() })
    req.pipe(upstream)
  }
}

module.exports = config
