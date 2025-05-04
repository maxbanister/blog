import type { Config, Context } from "@netlify/edge-functions";

// This edge function is used so we can respond early in the case the AP method
//  isn't supported, so that we don't incur the cost of a full function call.

export default async (req: Request, context: Context) => {
	// Save original body to pass to backend verbatim
	const text = await req.text();
	const body = JSON.parse(text);

	// If this causes an exception, let it fail and bypass the edge function
	if (body.type == "Delete") {
		const obj = body.object;
		if (typeof obj == "string" && obj.toLowerCase().includes("users")) {
			console.log(body);

			// Tell it the user is gone, so it stops pinging us
			return new Response(
				"202 Accepted",
				{
					status: 202,
					headers: { "content-type": "text/html" }
				}
			);
		}
	}

	context.waitUntil(logRequest());

	// It is necessary to replace the body which was just read out
	return context.next(new Request(req, { body: text }));
};

async function logRequest() {
	console.log("Spinning up");

	const resp = await fetch("https://maxbanister.com/posts/my-first-post/likes", {
	  method: "GET",
	  headers: { "Accept": "application/json" },
	});

	console.log(await resp.json());
  }

export const config: Config = {
	path: "/ap/inbox",
	onError: "bypass"
};