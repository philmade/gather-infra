/**
 * Gather Email Worker — Cloudflare Email Routing
 *
 * POST /  { to, subject, html, from?, fromName? }
 * Requires Authorization: Bearer <AUTH_TOKEN> header.
 * Uses Cloudflare Email Routing's SEND_EMAIL binding (free).
 */

import { EmailMessage } from "cloudflare:email";

const CORS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "POST, OPTIONS",
  "Access-Control-Allow-Headers": "Content-Type, Authorization",
};

export default {
  async fetch(request, env) {
    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: CORS });
    }

    if (request.method !== "POST") {
      return Response.json({ error: "Method not allowed" }, { status: 405, headers: CORS });
    }

    // Validate auth token
    const authHeader = request.headers.get("Authorization") || "";
    const token = authHeader.replace("Bearer ", "");
    if (!env.AUTH_TOKEN || token !== env.AUTH_TOKEN) {
      return Response.json({ error: "Unauthorized" }, { status: 401, headers: CORS });
    }

    try {
      const data = await request.json();

      if (!data.to || !data.subject || !data.html) {
        return Response.json(
          { error: "Missing required fields: to, subject, html" },
          { status: 400, headers: CORS }
        );
      }

      const fromEmail = data.from || "noreply@gather.is";
      const fromName = data.fromName || "Gather";

      const raw = [
        `From: ${fromName} <${fromEmail}>`,
        `To: ${data.to}`,
        `Subject: ${data.subject}`,
        `MIME-Version: 1.0`,
        `Content-Type: text/html; charset=UTF-8`,
        ``,
        data.html,
      ].join("\r\n");

      const message = new EmailMessage(fromEmail, data.to, raw);
      await env.SEND_EMAIL.send(message);

      return Response.json({ status: "sent" }, { status: 200, headers: CORS });
    } catch (error) {
      console.error("Email worker error:", error);
      return Response.json(
        { error: `Send failed: ${error.message}` },
        { status: 500, headers: CORS }
      );
    }
  },
};
