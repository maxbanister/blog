import type { Config, Context } from "@netlify/edge-functions";

// This edge function is used so we can respond early in the case the AP method
//  isn't supported, so that we don't incur the cost of a full function call.

export default async (req: Request, context: Context) => {
	console.log(await req.text());
	const body = await req.json();

	// If this causes an exception, just let it fail and bypass the edge function
	if (body.type == "Delete" && body.object.toLowerCase().includes("users")) {
		console.log(body);

		return new Response(
			"501 Not Implemented: unsupported operation",
			{
				status: 501,
				headers: { "content-type": "text/html" }
			}
		);
	}

	// It is necessary to replace the body which was just read out
	return context.next(new Request(req, { body: JSON.stringify(body) }));
};

export const config: Config = {
	path: "/ap/inbox",
	onError: "bypass"
};