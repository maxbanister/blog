import type { Context } from "@netlify/functions";
import * as crypto from "node:crypto";

export default async (req: Request, context: Context) => {
	const authHdr = req.headers.get("Authorization");
	const forbiddenResp = new Response(
		"403 Forbidden",
		{
			status: 403,
			headers: { "content-type": "text/html" }
		}
	);

	console.log("Auth hdr:", authHdr);
	console.log("Env var key:", process.env.SELF_API_KEY);

	if (!authHdr || !process.env.SELF_API_KEY) {
		return forbiddenResp;
	}

	const authHdrBuf = Buffer.from(authHdr);
	const selfAPIKeyBuf = Buffer.from(process.env.SELF_API_KEY);
	if (!crypto.timingSafeEqual(authHdrBuf, selfAPIKeyBuf)) {
		return forbiddenResp;
	}

	const body = await req.text();

	// don't stop function until follow service has sent AcceptRequest
	context.waitUntil(callFollowService(req, authHdr, body));

	return new Response("200 OK", { status: 200 });
};

async function callFollowService(req: Request, authHdr: string, body: string) {
	// give inbox function time to return 200 response
	await new Promise(resolve => setTimeout(resolve, 500));

	console.log("Calling follow service");

	await fetch(process.env.URL + "/.netlify/functions/follow-service", {
		method: "POST",
		body: body,
		headers: {
			"Content-Type": "application/json",
			"Authorization": authHdr
		},
	});
}
