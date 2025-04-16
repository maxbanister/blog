import type { Config, Context } from "@netlify/edge-functions";
import outbox from "../../public/ap/outbox.json" with { type: "json" };

// This edge function takes a request for a post's URL with an Accept type of
// activity+json and returns the item from the outbox that matches it.

export default async (req: Request, context: Context) => {
	// check if wants activity+json
	let acceptHeader = req.headers.get("Accept") || "";
	let wantsActivityJSON = acceptHeader.toLowerCase().includes("json");
	if (!wantsActivityJSON) {
		// continue request chain by returning undefined
		return;
	}

	console.log("Accept:", req.headers.get("Accept"));
	console.log("URL:", req.url);

	// find the item corresponding to this url within the outbox
	for (const item of outbox.orderedItems) {
		if (item.object.id == req.url) {
			console.log(JSON.stringify(item.object));
			return new Response(JSON.stringify(item.object), {
				headers: {"Content-Type": "application/activity+json"}
			});
		}
	}
};

export const config: Config = {
	path: "/posts/*",
	onError: "bypass"
};