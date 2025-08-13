const mastodonPrefix = "https://mastodon.social/authorize_interaction?uri=";

function colorHash(string) {
	let hash = 0;
	for (const char of string) {
		hash = (hash << 5) - hash + char.charCodeAt(0);
		hash |= 0; // Constrain to 32bit integer
	}
	const r = (hash >> 0) & 0x0ff;
	const g = (hash >> 8) & 0xff;
	const b = (hash >> 16) & 0xff;
	return "rgb(" + r + "," + g + "," + b +",0.5)";
}

// Use reversed=true when it gets supported
async function getBlueskyURL(handle, cursor, postURL) {
	console.log("fetching records for handle", handle+cursor);

	bridgyRequestURI = "https://atproto.brid.gy/xrpc/com.atproto.repo.listRecords?repo=";
	bridgyRequestURI += handle + "&collection=app.bsky.feed.post";
	if (cursor) {
		bridgyRequestURI += "&cursor=" + cursor;
	}

	const resp = await fetch(bridgyRequestURI, {
		headers: {"Accept": "application/json"}
	});
	const recordsJSON = await resp.json();

	console.log(recordsJSON);
	if (!recordsJSON) {
		console.log("could not get records for", handle+cursor);
		return;
	}
	for (const record of recordsJSON.records) {
		if (record.value.bridgyOriginalUrl == postURL) {
			const atURI = record.uri.replace("at://", "")
									.replace("app.bsky.feed.post", "post");
			return "https://bsky.app/profile/" + atURI;
		}
	}
	if (recordsJSON.cursor) {
		return getBlueskyURL(handle, recordsJSON.cursor);
	}
}

async function renderReplies() {
	const [mastodonAnchor, blueskyAnchor] = document.querySelectorAll("#social-links > a");
	mastodonAnchor.href = mastodonPrefix + window.location.href;
	blueskyAnchor.href = await getBlueskyURL(
		"maxbanister.com",
		"",
		"https://maxbanister.com" + window.location.pathname
	);

	const resp = await fetch(window.location.pathname + "replies");
	if (!resp.ok) {
		console.log(resp.statusText);
		return;
	}
	let repliesData = await resp.json();
	console.log(repliesData);

	addRepliesRecursive(document.getElementById("replies"), repliesData.items);
}

function addRepliesRecursive(parentEl, replyItems) {
	if (!replyItems)
		return;

	for (const item of replyItems) {
		const deleted = item.type == "Tombstone";
		item.url = deleted ? "javascript:void(0)"
		                   : item.url.replace("https://fed.brid.gy/r/", "");
		item.actor ||= {};

		const newReply = createAndAddReply(parentEl, {
			id: item.id,
			name: item.actor.name,
			shortName: item.actor.preferredUsername,
			host: deleted ? "" : new URL(item.actor.id).hostname,
			picURL: item.actor.icon,
			userURL: item.actor.id,
			date: item.published,
			editDate: item.updated,
			opURL: item.url,
			content: item.content
		}, deleted);

		addRepliesRecursive(newReply, item.replies.items);
	}
}

function createAndAddReply(parentEl, params, deleted) {
	const {
		id, name, shortName, host, picURL, userURL, date, editDate, opURL, content
	} = params;

	const options = {
		dateStyle: "long",
		timeStyle: "short"
	};

	let modifiedDate = new Intl.DateTimeFormat(undefined, options).format(new Date(date));
	if (editDate) {
		const dateEdited = new Intl.DateTimeFormat(undefined, options).format(new Date(editDate));
		modifiedDate += " (Edited: " + dateEdited + ")";
	}

	const template = document.getElementById("reply-template");
	const clone = template.content.cloneNode(true);
	const cloneEl = clone.firstElementChild;

	const [nameEl, hostEl] = clone.querySelectorAll(".reply-profile-info a span");
	nameEl.textContent = deleted ? "" : "@" + shortName;
	hostEl.textContent = deleted ? "" : "@" + host;

	const contentEl = cloneEl.getElementsByClassName("reply-contents")[0];
	contentEl.innerHTML = deleted ? "<i style=\"color: grey\">[deleted]</i>" : content;

	const profileImage = clone.querySelector(".reply-top > img");
	profileImage.src = deleted ? "" : "/image_proxy/" +
		encodeURIComponent(picURL) + "/" + encodeURIComponent(userURL);
	profileImage.style.backgroundColor = colorHash(nameEl.innerText + hostEl.innerText);
	if (deleted)
		profileImage.alt = "";

	const userAnchor = clone.querySelector(".reply-profile-info > a");
	userAnchor.href = userURL;

	const [nameSpan, dateSpan] = clone.querySelectorAll(".reply-profile-info > span");
	nameSpan.textContent = deleted ? "[deleted]" : name ? name : shortName;
	dateSpan.textContent = modifiedDate;

	const originalPostAnchor = clone.querySelector(".reply-op-button > a");
	originalPostAnchor.href = opURL;

	const [mastodonReplyBtn, blueskyReplyBtn] = clone.querySelectorAll(".reply-controls > a");
	if (new URL(opURL).host === "mastodon.social") {
		mastodonReplyBtn.href = opURL;
	}
	else {
		mastodonReplyBtn.href = mastodonPrefix + id;
	}
	if (deleted || host === "bsky.brid.gy") {
		blueskyReplyBtn.href = opURL;
	}
	else {
		const bridgedHandle = shortName + "." + host + ".ap.brid.gy";
		blueskyReplyBtn.addEventListener("click", async (e) => {
			console.log(blueskyReplyBtn.href);
			if (blueskyReplyBtn.getAttribute("href") === "#") {
				e.preventDefault();
				const newLoc = await getBlueskyURL(bridgedHandle, "", opURL);
				blueskyReplyBtn.href = newLoc;
				blueskyReplyBtn.click();
			}
		});
	}

	parentEl.appendChild(clone);
	return cloneEl;
}

async function main() {
	return renderReplies();
}

main();