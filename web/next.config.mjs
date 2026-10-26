/** Proxies /api to lp-server so the browser never needs CORS. */
const api = process.env.LP_API_URL ?? "http://localhost:8080";

export default {
  output: "standalone",
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${api}/api/:path*` }];
  },
};
