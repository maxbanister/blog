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

async function renderInteractions(typ) {
    const likesOrSharesEl = document.getElementById(typ);
    const labelEl = likesOrSharesEl.getElementsByTagName("label")[0];
    const expandedEl = document.querySelector("#" + typ + " + .expanded");

    const resp = await fetch(window.location.pathname + typ);
    if (!resp.ok) {
        console.log(resp.statusText);
        return;
    }
    let items = await resp.json();

    if (items.length == 0) {
        return;
    }
    likesOrSharesEl.style.display = "flex";
    expandedEl.style.display = "flex";
    const typCapitalized = typ.charAt(0).toUpperCase() + typ.slice(1);
    labelEl.firstChild.textContent = typCapitalized + " (" + items.length + ")";

    previewImages = likesOrSharesEl.getElementsByClassName("preview_images")[0];

    for (const [i, item] of items.entries()) {
        // Mastodon's boost URL redirects you to the original post, i.e. this very page.
        // This is unhelpful, so we will instead link to the user's profile.
        const url = item.actor.id;
        const actorHost = new URL(item.actor.id).hostname;
        const actorName = item.actor.preferredUsername;
        const handle = "@" + actorName + "@" + actorHost;
        const imgSrc = "/image_proxy/" +
            encodeURIComponent(item.actor.icon) + "/" +
            encodeURIComponent(item.actor.id);

        const aPreview = document.createElement("a");
        aPreview.href = url;
        aPreview.title = handle;
        let img = document.createElement("img");
        img.setAttribute("src", imgSrc);
        img.setAttribute("alt", actorName[0]);
        img.setAttribute("width", "32");
        img.setAttribute("height", "32");
        const r = actorName[0].charCodeAt(0);
        const g = actorName[1].charCodeAt(0);
        const b = actorName[2].charCodeAt(0);
        img.style.backgroundColor = colorHash(handle);
        aPreview.appendChild(img);

        if (i <= 3) {
            previewImages.appendChild(aPreview);
        }
        if (i == 3) {
            previewImages.appendChild(document.createTextNode("\u00A0…"));
        }

        // <span><img src="…" alt="…"/><a href="…">name</a></span>
        const a = document.createElement("a");
        const span = document.createElement("span");
        img = img.cloneNode();
        a.href = url;
        a.title = handle;
        span.appendChild(img);
        span.append(actorName);
        span.style.display = "inline-block";
        a.appendChild(span);

        expandedEl.appendChild(a);
    }
}

async function main() {
    return Promise.all[
        renderInteractions("likes"),
        renderInteractions("shares")
    ];
}

main();
