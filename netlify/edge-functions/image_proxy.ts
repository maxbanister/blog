import type { Config, Context } from "@netlify/edge-functions";

// This edge function proxies image requests from the user's client to the
// social media site it's hosted on. Also, if it 404's, we will call the refresh
// service to refresh the user profile belonging to the fetched image and return
// the updated profile pic

export default async (req: Request, context: Context) => {
	const parts = req.url.split("image_proxy/").pop()?.split("/");
	if (!parts || parts.length !== 3) {
		return;
	}
	const [iconURL, colName, refID] = parts;
	if (!colName || !iconURL || !refID ||
		["likes", "shares", "replies"].indexOf(colName) == -1
	) {
		return new Response("Bad Request", {"status": 400});
	}

	try {
		const origResp = await fetch(decodeURIComponent(iconURL));
		if (origResp.status === 200) {
			return origResp;
		}
	} catch (error) {
		// do nothing
	}

	console.log("Original image link rotten, fetching new - ", iconURL, refID);

	const refreshResp = await fetch(
		context.site.url +
		"/.netlify/functions/refresh-profile?iconURL=" + iconURL +
		"&colName=" + colName +
		"&refID=" + refID,
		{
			method: "GET",
			headers: {
				"Content-Type": "application/json",
				"Authorization": "" + Netlify.env.get("SELF_API_KEY")
			},
		}
	);
	if (refreshResp.status !== 200) {
		return refreshResp;
	}

	return fetch(await refreshResp.text());
};

export const config: Config = {
	path: "/image_proxy/*"
};