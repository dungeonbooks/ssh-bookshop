import llms from "../llms.txt";

// One file on the storefront hostnames: the llms.txt an agent probes for
// first. It names the docs site and the shop so the agent learns both exist.
export default {
  async fetch(request): Promise<Response> {
    try {
      if (new URL(request.url).pathname === "/llms.txt") {
        if (request.method !== "GET" && request.method !== "HEAD") {
          return new Response("method not allowed", { status: 405, headers: { allow: "GET, HEAD" } });
        }
        const response = new Response(llms, {
          headers: {
            "content-type": "text/markdown; charset=utf-8",
            "cache-control": "public, max-age=300",
            "x-content-type-options": "nosniff",
            link: '</llms.txt>; rel="llms-txt"',
          },
        });
        return request.method === "HEAD" ? new Response(null, response) : response;
      }
    } catch (err) {
      // Fail open: the storefront must answer even if this Worker cannot.
      console.error(JSON.stringify({ event: "edge_error", message: String(err) }));
    }
    return fetch(request);
  },
} satisfies ExportedHandler<Env>;
