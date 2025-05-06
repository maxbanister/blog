(() => {
  // <stdin>
  async function renderReplies() {
    const mastodonAnchor = document.querySelector("#social-links > a");
    mastodonAnchor.href = "https://mastodon.social/authorize_interaction?uri=" + window.location.href;
    const resp = await fetch(window.location.pathname + "replies");
    if (!resp.ok) {
      console.log(resp.statusText);
      return;
    }
    let repliesData = await resp.json();
    console.log(repliesData);
    addRepliesRecursive(document.getElementById("replies"), repliesData.items);
    let p = {
      name: "max",
      host: "congress.gov",
      picURL: "https://external-content.duckduckgo.com/iu/?u=http%3A%2F%2Fih1.redbubble.net%2Fimage.186232319.8599%2Fsticker%2C375x360.u1.png&f=1&nofb=1&ipt=78c48edebb516b832dc4bb4c3cf1bf8ba11f110e1c8718de68dbdbb829e4f151",
      userURL: "https://maxbanister.com",
      opURL: "https://maxbanister.com/post",
      content: "Hello World"
    };
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
        content: item.content,
        linkBackURL: item.url
      });
      addRepliesRecursive(newReply, item.replies.items);
    }
  }
  function createAndAddReply(parentEl, params) {
    const {
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
    parentEl.appendChild(clone);
    return cloneEl;
  }
  async function main() {
    return renderReplies();
  }
  main();
})();
