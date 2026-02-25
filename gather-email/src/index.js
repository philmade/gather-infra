/**
 * Gather Email Worker — Cloudflare Email Routing
 *
 * Outbound: POST /  { to, subject, html, from?, fromName? }
 * Inbound:  email() handler — Cloudflare Email Routing catch-all
 *
 * Requires Authorization: Bearer <AUTH_TOKEN> header for outbound.
 * Uses SEND_EMAIL binding (free) for outbound, GATHER_AUTH_URL for inbound delivery.
 */

import { EmailMessage } from "cloudflare:email";

const CORS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "POST, OPTIONS",
  "Access-Control-Allow-Headers": "Content-Type, Authorization",
};

const FALLBACK_ADDR = "phil@imrge.co";

export default {
  // --- Outbound: send email via SEND_EMAIL binding ---
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
      const msgId = `<${Date.now()}.${Math.random().toString(36).slice(2)}@gather.is>`;

      const raw = [
        `From: ${fromName} <${fromEmail}>`,
        `To: ${data.to}`,
        `Subject: ${data.subject}`,
        `Message-ID: ${msgId}`,
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

  // --- Inbound: Cloudflare Email Routing delivers here ---
  async email(message, env) {
    const to = message.to;
    const from = message.from;

    // Only handle @gather.is addresses
    const match = to.match(/^([^@]+)@gather\.is$/i);
    if (!match) {
      await message.forward(FALLBACK_ADDR);
      return;
    }

    const headers = message.headers;
    const subject = headers.get("subject") || "(no subject)";
    const messageId = headers.get("message-id") || "";
    const inReplyTo = headers.get("in-reply-to") || "";

    // Read raw email text (simplified — full MIME parsing is v2)
    let bodyText = "";
    try {
      const raw = await new Response(message.raw).text();
      // Extract body after the blank line separating headers from body
      const bodyStart = raw.indexOf("\r\n\r\n");
      bodyText = bodyStart >= 0 ? raw.slice(bodyStart + 4) : raw;
      // Limit size
      if (bodyText.length > 10000) bodyText = bodyText.slice(0, 10000);
    } catch (e) {
      console.error("Failed to read email body:", e);
    }

    // Deliver to gather-auth
    const authURL = env.GATHER_AUTH_URL || "https://gather.is";
    try {
      const resp = await fetch(authURL + "/api/email/inbound", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          secret: env.EMAIL_INBOUND_SECRET,
          from_addr: from,
          to_addr: to,
          subject: subject,
          body_html: "",
          body_text: bodyText,
          message_id: messageId,
          in_reply_to: inReplyTo,
        }),
      });

      if (!resp.ok) {
        console.error(`Inbound delivery failed: ${resp.status} ${await resp.text()}`);
        await message.forward(FALLBACK_ADDR);
      }
    } catch (e) {
      console.error("Inbound delivery error:", e);
      await message.forward(FALLBACK_ADDR);
    }
  },
};
