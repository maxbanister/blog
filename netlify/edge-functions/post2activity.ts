import type { Config, Context } from "@netlify/edge-functions";
import * as outbox from "../../public/ap/outbox";

// This edge function takes a request for a post's URL with an Accept type of
// activity+json and returns the item from the outbox that matches it.

export default async (req: Request, context: Context) => {
	// check if wants activity+json
	let acceptHeader = req.headers.get("Accept") || "";
	let wantsActivityJSON = acceptHeader.includes('application/activity+json');
	if (!wantsActivityJSON)
		// continue request chain by returning undefined
		return;

	// find the item corresponding to this url within the outbox
	for (const post of outbox.ordereredItems) {
		if (post.object.id == req.url) {
			return new Response(JSON.stringify(post), {
				headers: {"Content-Type": "application/activity+json"}
			});
		}
	}
};

export const config: Config = {
	path: "/posts",
	onError: "bypass"
};