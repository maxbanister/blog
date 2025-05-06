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
        const url = item.url
                        ? item.url
                        : typ == "likes" ? item.id : item.actor.id;
        const actorHost = new URL(item.actor.id).hostname;
        const actorName = item.actor.preferredUsername;
        const handle = "@" + actorName + "@" + actorHost;
        const imgSrc = item.actor.icon;

        const aPreview = document.createElement("a");
        aPreview.href = url;
        aPreview.title = handle;
        let img = document.createElement("img");
        img.setAttribute("src", imgSrc);
        img.setAttribute("alt", actorName);
        img.setAttribute("width", "32");
        img.setAttribute("height", "32");
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
