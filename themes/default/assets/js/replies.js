const mastodonPrefix = "https://mastodon.social/authorize_interaction?uri=";

async function renderReplies() {
	const mastodonAnchor = document.querySelector("#social-links > a");
	mastodonAnchor.href = mastodonPrefix + window.location.href;

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

	for (item of replyItems) {
		const newReply = createAndAddReply(parentEl, {
			name: item.actor.name,
			shortName: item.actor.preferredUsername,
			host: new URL(item.actor.id).hostname,
			picURL: item.actor.icon,
			userURL: item.actor.id,
			date: item.published,
			editDate: item.updated,
			opURL: item.id,
			content: item.content
		});

		addRepliesRecursive(newReply, item.replies.items);
	}
}

function createAndAddReply(parentEl, params) {
	const {
		name, shortName, host, picURL, userURL, date, editDate, opURL, content
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
	nameEl.textContent = "@" + shortName;
	hostEl.textContent = "@" + host;

	const contentEl = cloneEl.getElementsByClassName("reply-contents")[0];
	contentEl.innerHTML = content;

	const profileImage = clone.querySelector(".reply-top > img");
	profileImage.src = picURL;

	const userAnchor = clone.querySelector(".reply-profile-info > a");
	userAnchor.href = userURL;

	const [nameSpan, dateSpan] = clone.querySelectorAll(".reply-profile-info > span");
	nameSpan.textContent = name ? name : shortName;
	dateSpan.textContent = modifiedDate;

	const originalPostAnchor = clone.querySelector(".reply-op-button > a");
	originalPostAnchor.href = opURL;

	const [mastodonReplyBtn, ...rest] = clone.querySelectorAll(".reply-controls a");
	if (new URL(opURL).host == "mastodon.social") {
		mastodonReplyBtn.href = opURL;
	}
	else {
		mastodonReplyBtn.href = mastodonPrefix + opURL;
	}

	parentEl.appendChild(clone);
	return cloneEl;
}

async function main() {
	return renderReplies();
}

main();