(() => {
  // <stdin>
  var mastodonPrefix = "https://mastodon.social/authorize_interaction?uri=";
  async function getBlueskyURL(handle, cursor, postURL) {
    console.log("fetching records for handle", handle + cursor);
    bridgyRequestURI = "https://atproto.brid.gy/xrpc/com.atproto.repo.listRecords?repo=";
    bridgyRequestURI += handle + "&collection=app.bsky.feed.post";
    if (cursor) {
      bridgyRequestURI += "&cursor=" + cursor;
    }
    const resp = await fetch(bridgyRequestURI, {
      headers: { "Accept": "application/json" }
    });
    const recordsJSON = await resp.json();
    console.log(recordsJSON);
    if (!recordsJSON) {
      console.log("could not get records for", handle + cursor);
      return;
    }
    for (const record of recordsJSON.records) {
      if (record.value.bridgyOriginalUrl == postURL) {
        const atURI = record.uri.replace("at://", "").replace("app.bsky.feed.post", "post");
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
    addRepliesRecursive(document.getElementById("replies"), repliesData.items);
  }
  function addRepliesRecursive(parentEl, replyItems) {
    if (!replyItems)
      return;
    for (const item of replyItems) {
      item.url = item.url.replace("https://fed.brid.gy/r/", "");
      const newReply = createAndAddReply(parentEl, {
        id: item.id,
        name: item.actor.name,
        shortName: item.actor.preferredUsername,
        host: new URL(item.actor.id).hostname,
        picURL: item.actor.icon,
        userURL: item.actor.id,
        date: item.published,
        editDate: item.updated,
        opURL: item.url,
        content: item.content
      });
      addRepliesRecursive(newReply, item.replies.items);
    }
  }
  function createAndAddReply(parentEl, params) {
    const {
      id,
      name,
      shortName,
      host,
      picURL,
      userURL,
      date,
      editDate,
      opURL,
      content
    } = params;
    const options = {
      dateStyle: "long",
      timeStyle: "short"
    };
    let modifiedDate = new Intl.DateTimeFormat(void 0, options).format(new Date(date));
    if (editDate) {
      const dateEdited = new Intl.DateTimeFormat(void 0, options).format(new Date(editDate));
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
    const [mastodonReplyBtn, blueskyReplyBtn] = clone.querySelectorAll(".reply-controls > a");
    if (new URL(opURL).host === "mastodon.social") {
      mastodonReplyBtn.href = opURL;
    } else {
      mastodonReplyBtn.href = mastodonPrefix + id;
    }
    if (host === "bsky.brid.gy") {
      blueskyReplyBtn.href = opURL;
    } else {
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
})();
