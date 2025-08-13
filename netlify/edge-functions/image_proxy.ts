import type { Config, Context } from "@netlify/edge-functions";

// This edge function proxies image requests from the user's client to the
// social media site it's hosted on. Also, if it 404's, we will call the refresh
// service to refresh the user profile belonging to the fetched image and return
// the updated profile pic

export default async (req: Request, context: Context) => {
	const parts = req.url.split("image_proxy/").pop()?.split("/");
	if (!parts || parts.length !== 2) {
		return;
	}
	const destURL = decodeURIComponent(parts[0]);
	const actorID = decodeURIComponent(parts[1]);
	if (!destURL) {
		return;
	}

	const origResp = await fetch(destURL);
	if (origResp.status === 200) {
		return origResp;
	}

	console.log("Original image link rotten, fetching new - " + actorID);

	const refreshResp = await fetch(
		context.site.url +
		"/.netlify/functions/refresh-profile?actorID=" +
		actorID,
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