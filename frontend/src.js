import "./style.css";
const api = () => window.go?.main?.App,
  $ = (s) => document.querySelector(s),
  id = () => Date.now().toString() + Math.random().toString(16).slice(2),
  esc = (s) =>
    String(s ?? "").replace(
      /[&<>\"]/g,
      (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c],
    );
let store = { groups: [], channels: [], lastGroupId: "", lastChannelId: "", lastNoteId: "", theme: "dark" },
  saveTimer,
  dirty = false,
  selectedImage = null,
  notesCollapsed = false,
  selectMode = false,
  selectedNotes = new Set(),
  askResolve = null;
let closeHandlerRegistered=false;
const channel = () =>
    store.channels.find((c) => c.id === store.lastChannelId) ||
    store.channels[0],
  group = () => store.groups.find((g) => g.id === store.lastGroupId) || store.groups[0],
  visibleChannels = () => store.channels.filter((c) => c.groupId === store.lastGroupId),
  allNotes = () => (channel()?.categories || []).flatMap((g) => g.notes || []),
  category = () =>
    channel()?.categories.find((g) => g.id === store.lastCategoryId) ||
    channel()?.categories[0],
  note = () =>
    allNotes().find((n) => n.id === store.lastNoteId) || allNotes()[0];
const newNote = () => ({
  id: id(),
  title: "새 메모",
  name: "새 메모",
  titleLinked: true,
  content: "<p></p>",
  contentLoaded: true,
});
function withoutLegacyTitle(content,title){const box=document.createElement("div");box.innerHTML=content||"";const first=box.firstElementChild;if(first?.tagName==="H1"&&first.textContent.trim()===(title||"").trim())first.remove();return box.innerHTML||"<p></p>"}
$("#app").innerHTML = `<div class="shell"><aside class="rail"><button class="settings-icon" id="openSettings" title="설정">⚙</button><div id="channels"></div><button class="round" id="addChannel">＋</button></aside><aside class="sidebar"><header><strong id="channelName"></strong></header><div class="section-head"><button id="toggleNotes">⌄ 메모장</button><div><button id="bulkDelete" title="여러 메모 삭제">🗑</button><button id="addNote">＋</button></div></div><div id="bulkBar"><span id="selectedCount">0개 선택</span><button id="deleteSelected">삭제</button><button id="cancelSelect">취소</button></div><div id="notes"></div></aside><main><header class="top"><input id="title"><button id="export">내보내기</button></header><div class="toolbar"><button data-cmd="bold"><b>B</b></button><button data-cmd="italic"><i>I</i></button><button data-cmd="underline"><u>U</u></button><select id="textColor" title="텍스트 색상"><option value="default">기본색</option><option value="red">빨강</option><option value="orange">주황</option><option value="green">초록</option><option value="blue">파랑</option><option value="purple">보라</option></select><select id="font"></select><div class="size-box"><input id="size" type="number" min="6" max="144" value="10"><span>pt</span><div class="size-presets">${[8,9,10,11,12,14,16,18,20,24].map(x=>`<button data-size="${x}">${x}</button>`).join("")}</div></div><button id="image">이미지 삽입</button><div id="imageTools"><input id="imageWidth" type="number" min="5" max="100" value="100"><span>%</span><button data-align="left">왼쪽</button><button data-align="center">가운데</button><button data-align="right">오른쪽</button></div></div><div class="editor-wrap"><div id="editor" contenteditable="true" spellcheck="true"></div><div id="resizeBox"><i data-handle="nw"></i><i data-handle="ne"></i><i data-handle="sw"></i><i data-handle="se"></i></div></div><footer><span id="status">준비됨</span><span>이미지 붙여넣기·드래그 지원</span></footer></main></div><div id="context" class="context-menu"></div><div id="modal" class="modal"><div class="dialog"><header><h2>설정</h2><button id="closeSettings">×</button></header><section><label>테마</label><select id="theme"><option value="system">시스템 설정에 따르기</option><option value="dark">어두운 테마</option><option value="light">밝은 테마</option></select></section><section><h3>데이터 관리</h3><div class="settings-actions"><button id="backup">전체 백업</button><button id="restore">전체 복원</button></div></section></div></div><div id="askModal" class="modal"><div class="dialog ask-dialog"><header><h2 id="askTitle"></h2></header><section><p id="askMessage"></p><input id="askInput"><div class="ask-actions"><button id="askCancel">취소</button><button id="askOK">확인</button></div></section></div></div>`;
$("#export").textContent = "다른 위치에 저장";
$("#export").insertAdjacentHTML("beforebegin", '<button id="import">불러오기</button>');
$("#import").insertAdjacentHTML("beforebegin", '<button id="manualSave">저장</button>');
$("footer span:last-child").remove();
$("#addNote").title = "카테고리 추가";
$(".settings-actions").insertAdjacentHTML("afterend",'<div class="group-data"><label>그룹 백업·복원</label><select id="groupDataSelect"></select><div><button id="backupGroup">선택 그룹 백업</button><button id="restoreGroup">그룹 복원</button></div></div>');
$("#openSettings").insertAdjacentHTML("afterend", '<button class="group-button" id="groupButton" title="그룹 선택"></button>');
document.body.insertAdjacentHTML("beforeend", '<div id="groupPopover" class="group-popover"></div>');
$("#theme").parentElement.insertAdjacentHTML("beforeend",'<label class="setting-check"><input id="showGroupPopup" type="checkbox"> 실행 시 그룹 선택 팝업 표시</label>');
document.body.insertAdjacentHTML("beforeend",'<div id="startupGroupModal" class="modal"><div class="dialog group-dialog"><header><h2>작업할 그룹 선택</h2></header><section><p>이 창에서 편집할 그룹을 선택하세요.</p><div id="startupGroups"></div></section></div></div><div id="imageEditModal" class="modal"><div class="dialog image-dialog"><header><h2>이미지 편집하기</h2><button id="closeImageEdit">×</button></header><section><div class="crop-stage"><img id="cropImage"></div><div class="zoom-row"><span>▧</span><input id="cropZoom" type="range" min="100" max="400" value="100"><span>▣</span></div><div class="crop-actions"><button id="cropReset">재설정</button><span></span><button id="cropCancel">취소</button><button id="cropApply">적용하기</button></div></section></div></div>');
function normalize() {
  if (!store.groups?.length) store.groups=[{id:id(),name:"기본 그룹"}];
  if (!store.groups.some(g=>g.id===store.lastGroupId)) store.lastGroupId=store.groups[0].id;
  if (!store.channels?.length) {
    store.channels = [{ id:id(), name:"내 채널", image:"", groupId:store.lastGroupId, categories:[{id:id(),name:"메모장",notes:[newNote()]}] }];
    store.lastChannelId = store.channels[0].id;
  }
  store.theme ||= "dark";
  for (const c of store.channels) {
    c.image ??= "";
    c.groupId ||= store.groups[0].id;
    if (!c.categories?.length) c.categories = [{ id: id(), name: "메모장", notes: c.notes || [] }];
    delete c.notes;
    for (const g of c.categories) { g.notes ??= []; for (const n of g.notes) { if (!n.name) { n.name=n.title||"제목 없음"; n.titleLinked=true; } n.titleLinked??=true; } }
  }
  if(!visibleChannels().length){const c={id:id(),name:"내 채널",image:"",groupId:store.lastGroupId,categories:[{id:id(),name:"메모장",notes:[newNote()]}]};store.channels.push(c)}
  if(!visibleChannels().some(c=>c.id===store.lastChannelId))store.lastChannelId=visibleChannels()[0].id;
  const c = channel();
  if (!c.categories.length) c.categories=[{id:id(),name:"메모장",notes:[newNote()]}];
  if (!c.categories.some((g)=>g.id===store.lastCategoryId)) store.lastCategoryId=c.categories[0].id;
  if (!allNotes().length) category().notes.push(newNote());
  if (!allNotes().some((n) => n.id === store.lastNoteId)) store.lastNoteId = allNotes()[0].id;
}
function applyTheme() {
  document.documentElement.dataset.theme = store.theme;
  $("#theme").value = store.theme;
  $("#showGroupPopup").checked=store.showGroupPopup!==false;
}
function render() {
  normalize();
  const c = channel(),
    n = note();
  applyTheme();
  $("#groupButton").textContent=group().name.slice(0,3);
  $("#groupButton").title=group().name;
  $("#groupDataSelect").innerHTML=store.groups.map(g=>`<option value="${g.id}" ${g.id===store.lastGroupId?'selected':''}>${esc(g.name)}</option>`).join('');
  $("#channels").innerHTML = visibleChannels()
    .map(
      (x) =>
        `<button class="channel ${x.id === c.id ? "active" : ""}" data-id="${x.id}" title="${esc(x.name)}">${x.image ? `<img src="${x.image}">` : esc(x.name.slice(0, 2))}</button>`,
    )
    .join("");
  $("#channelName").textContent = c.name;
  $("#notes").classList.toggle("collapsed", notesCollapsed);
  $("#toggleNotes").textContent = `${notesCollapsed ? "›" : "⌄"} 카테고리`;
  $("#bulkBar").classList.toggle("show", selectMode);
  $("#selectedCount").textContent = `${selectedNotes.size}개 선택`;
  $("#notes").innerHTML = c.categories.map(g=>`<section class="category ${g.id===store.lastCategoryId?'current':''}" data-category-id="${g.id}"><div class="category-head"><button class="category-name">⌄ ${esc(g.name)}</button><button class="category-add" title="이 카테고리에 메모 추가">＋</button></div><div class="category-notes">${g.notes.map(x=>selectMode?`<label class="note selectable" data-id="${x.id}"><input type="checkbox" ${selectedNotes.has(x.id)?"checked":""}><span>#</span><b>${esc(x.name||x.title)}</b></label>`:`<button class="note ${x.id===n.id?"active":""}" data-id="${x.id}"><span>#</span><b>${esc(x.name||x.title)}</b></button>`).join("")}</div></section>`).join("");
  $("#title").value = n.title || "";
  if(n.contentLoaded)n.content=withoutLegacyTitle(n.content,n.title);
  $("#editor").innerHTML = n.content || "";
  ensureNoteLoaded(n.id);
  selectedImage = null;
  imageTools();
  document.querySelectorAll(".channel").forEach((b) => {
    b.onclick = () => switchChannel(b.dataset.id);
    b.oncontextmenu = (e) => channelMenu(e, b.dataset.id);
  });
  document.querySelectorAll(".note").forEach((b) => {
    if (selectMode) b.onclick = () => toggleSelected(b.dataset.id);
    else {
      b.onclick = () => switchNote(b.dataset.id);
      b.oncontextmenu = (e) => noteMenu(e, b.dataset.id);
    }
  });
  document.querySelectorAll(".category").forEach((el)=>{
    const gid=el.dataset.categoryId;
    el.querySelector(".category-name").onclick=()=>{store.lastCategoryId=gid;el.classList.toggle("folded");mark()};
    el.querySelector(".category-name").oncontextmenu=(e)=>categoryMenu(e,gid);
    el.querySelector(".category-add").onclick=()=>addNote(gid);
  });
}
async function switchChannel(cid) {
  await save();
  for(const n of allNotes()){n.content="";n.contentLoaded=false}
  store.lastChannelId = cid;
  store.lastCategoryId = channel().categories[0]?.id || "";
  store.lastNoteId = allNotes()[0]?.id || "";
  render();
  mark();
}
async function switchNote(nid) {
  await save();
  const previous=note();if(previous&&previous.id!==nid){previous.content="";previous.contentLoaded=false}
  store.lastNoteId = nid;
  const owner=channel().categories.find(g=>g.notes.some(n=>n.id===nid));
  if(owner) store.lastCategoryId=owner.id;
  render();
  mark();
}
function capture() {
  const n = note();
  if (!n) return;
  n.title = $("#title").value.trim() || "제목 없음";
  if(n.contentLoaded) n.content = $("#editor").innerHTML;
  if (n.titleLinked) n.name = n.title;
  dirty = true;
}
async function ensureNoteLoaded(nid) {
  const target=allNotes().find(n=>n.id===nid);if(!target||target.contentLoaded)return;
  try{const content=await api().LoadNoteContent(nid);if(note()?.id!==nid)return;target.content=withoutLegacyTitle(content,target.title);target.contentLoaded=true;$("#editor").innerHTML=target.content;if(target.content!==content)mark()}catch(e){$("#status").textContent="메모 로딩 실패: "+e}
}
function mark() {
  dirty = true;
  clearTimeout(saveTimer);
  saveTimer = setTimeout(save, 1200);
}
async function save() {
  if (!api() || !dirty) return;
  capture();
  try {
    const t = await api().SaveStore(store);
    dirty = false;
    $("#status").textContent = `${t} · 자동저장 완료`;
  } catch (e) {
    $("#status").textContent = "저장 실패: " + e;
  }
}
setInterval(() => dirty && save(), 5000);
addEventListener("beforeunload", save);
function ask(title, message, value) {
  $("#askTitle").textContent = title;
  $("#askMessage").textContent = message || "";
  $("#askInput").style.display = value === undefined ? "none" : "block";
  $("#askInput").value = value ?? "";
  $("#askModal").classList.add("show");
  if (value !== undefined) setTimeout(() => $("#askInput").select(), 0);
  return new Promise((resolve) => (askResolve = resolve));
}
function closeAsk(value) {
  $("#askModal").classList.remove("show");
  askResolve?.(value);
  askResolve = null;
}
$("#askOK").onclick = () => closeAsk($("#askInput").style.display === "none" ? true : $("#askInput").value);
$("#askCancel").onclick = () => closeAsk(null);
$("#askInput").onkeydown = (e) => { if (e.key === "Enter") $("#askOK").click(); };
function menu(e, items) {
  e.preventDefault();
  const m = $("#context");
  m.innerHTML = items
    .map((x, i) =>
      x.sep
        ? "<hr>"
        : `<button data-i="${i}" class="${x.danger ? "danger" : ""}">${x.check !== undefined ? `<span>${x.check ? "✓" : " "}</span>` : ""}${esc(x.label)}</button>`,
    )
    .join("");
  m.style.display = "block";
  m.style.left = Math.min(e.clientX, innerWidth - 230) + "px";
  m.style.top = Math.min(e.clientY, innerHeight - m.offsetHeight - 10) + "px";
  m.querySelectorAll("button").forEach(
    (b) =>
      (b.onclick = () => {
        items[+b.dataset.i].run();
        $("#context").style.display = "none";
      }),
  );
}
document.addEventListener("click", (e) => {
  if (!e.target.closest("#context")) $("#context").style.display = "none";
  if (!e.target.closest("#groupPopover,#groupButton")) $("#groupPopover").classList.remove("show");
});
async function renderGroupPopover() {
  const p=$("#groupPopover");
  const locked=new Set(await api().LockedGroups());
  p.innerHTML=`<div class="group-title">채널 그룹</div>${store.groups.map(g=>`<div class="group-row ${g.id===store.lastGroupId?'active':''} ${locked.has(g.id)?'locked':''}" data-id="${g.id}"><button class="group-select" ${locked.has(g.id)?'disabled':''}><b>${esc(g.name)}</b><small>${locked.has(g.id)?'다른 창에서 작업 중':store.channels.filter(c=>c.groupId===g.id).length+'개 채널'}</small></button><button class="group-edit" title="명칭 수정" ${locked.has(g.id)?'disabled':''}>✎</button><button class="group-delete" title="삭제" ${locked.has(g.id)?'disabled':''}>×</button></div>`).join('')}<button id="addGroup" class="add-group">＋ 새 그룹</button>`;
  p.querySelectorAll('.group-row').forEach(row=>{const gid=row.dataset.id;row.querySelector('.group-select').onclick=()=>selectGroup(gid);row.querySelector('.group-edit').onclick=()=>renameGroup(gid);row.querySelector('.group-delete').onclick=()=>deleteGroup(gid)});
  $('#addGroup').onclick=addGroup;
}
$("#groupButton").onclick=async(e)=>{e.stopPropagation();await renderGroupPopover();$("#groupPopover").classList.toggle("show")};
async function selectGroup(gid){await save();try{await api().AcquireGroup(gid)}catch(e){return ask("선택할 수 없음",String(e))}store=await api().ReloadStore();normalize();store.lastGroupId=gid;const c=visibleChannels()[0];store.lastChannelId=c?.id||'';store.lastCategoryId=c?.categories[0]?.id||'';store.lastNoteId=c?.categories.flatMap(g=>g.notes)[0]?.id||'';$("#groupPopover").classList.remove("show");$("#startupGroupModal").classList.remove("show");render();mark()}
async function addGroup(){const v=await ask("새 그룹","그룹 이름을 입력하세요.","새 그룹");if(!v?.trim())return;const g={id:id(),name:v.trim()};store.groups.push(g);store.lastGroupId=g.id;$("#groupPopover").classList.remove("show");render();mark();await selectGroup(g.id)}
async function renameGroup(gid){if(gid!==store.lastGroupId)await selectGroup(gid);const g=store.groups.find(x=>x.id===gid),v=await ask("그룹 명칭 수정","새 그룹 이름을 입력하세요.",g.name);if(v?.trim()){g.name=v.trim();render();await renderGroupPopover();mark()}}
async function deleteGroup(gid){if(store.groups.length===1)return ask("삭제할 수 없음","마지막 그룹은 삭제할 수 없습니다.");if(gid!==store.lastGroupId)await selectGroup(gid);const g=store.groups.find(x=>x.id===gid),count=store.channels.filter(c=>c.groupId===gid).length;if(!(await ask("그룹 삭제",`‘${g.name}’ 그룹과 포함된 채널 ${count}개를 삭제할까요?`)))return;store.groups=store.groups.filter(x=>x.id!==gid);store.channels=store.channels.filter(c=>c.groupId!==gid);const next=store.groups[0].id;store.lastGroupId=next;$("#groupPopover").classList.remove("show");render();mark();await save();await selectGroup(next)}
function channelMenu(e, cid) {
  const c = store.channels.find((x) => x.id === cid);
  menu(e, [
    {
      label: "채널 이름 수정",
      run: async () => {
        const v = await ask("채널 이름 수정", "새 채널 이름을 입력하세요.", c.name);
        if (v?.trim()) {
          c.name = v.trim();
          render();
          mark();
        }
      },
    },
    {
      label: "채널 이미지 설정",
      run: async () => {
        const a = await api().SelectImages();
        if (a?.[0]) {
          const edited=await editChannelImage(a[0].dataUrl);
          if(edited){c.image=edited;render();mark()}
        }
      },
    },
    {
      label: "채널 이미지 제거",
      run: () => {
        c.image = "";
        render();
        mark();
      },
    },
    { sep: true },
    { label: "채널 삭제", danger: true, run: () => deleteChannel(cid) },
  ]);
}
function noteMenu(e, nid) {
  const n = allNotes().find((x) => x.id === nid);
  menu(e, [
    {
      label: "메모장 명칭 수정",
      run: async () => {
        const v = await ask("메모장 명칭 수정", "목록에 표시할 명칭을 입력하세요.", n.name);
        if (v?.trim()) {
          n.name = v.trim();
          n.titleLinked = false;
          render();
          mark();
        }
      },
    },
    {
      label: "제목과 명칭 연동",
      check: n.titleLinked,
      run: () => {
        n.titleLinked = !n.titleLinked;
        if (n.titleLinked) n.name = n.title;
        render();
        mark();
      },
    },
    { sep: true },
    { label: "메모장 삭제", danger: true, run: () => deleteNote(nid) },
  ]);
}
function categoryMenu(e, gid) {
  const g=channel().categories.find(x=>x.id===gid);
  menu(e,[{label:"카테고리 이름 수정",run:async()=>{const v=await ask("카테고리 이름 수정","새 이름을 입력하세요.",g.name);if(v?.trim()){g.name=v.trim();render();mark()}}},{sep:true},{label:"카테고리 삭제",danger:true,run:()=>deleteCategory(gid)}]);
}
async function deleteCategory(gid) {
  const c=channel(),g=c.categories.find(x=>x.id===gid);
  if(c.categories.length===1) return ask("삭제할 수 없음","마지막 카테고리는 삭제할 수 없습니다.");
  if(!(await ask("카테고리 삭제",`‘${g.name}’ 카테고리와 포함된 메모 ${g.notes.length}개를 삭제할까요?`)))return;
  c.categories=c.categories.filter(x=>x.id!==gid);
  if(store.lastCategoryId===gid)store.lastCategoryId=c.categories[0].id;
  if(g.notes.some(n=>n.id===store.lastNoteId))store.lastNoteId=allNotes()[0]?.id||"";
  render();mark();
}
async function deleteChannel(cid) {
  if (visibleChannels().length === 1)
    return ask("삭제할 수 없음", "그룹의 마지막 채널은 삭제할 수 없습니다.");
  if (!(await ask("채널 삭제", "이 채널과 모든 메모장을 삭제할까요?"))) return;
  store.channels = store.channels.filter((x) => x.id !== cid);
  if (store.lastChannelId === cid) {
    const next=visibleChannels()[0];
    store.lastChannelId = next.id;
    store.lastCategoryId = next.categories[0]?.id || "";
    store.lastNoteId = next.categories.flatMap(g=>g.notes)[0]?.id || "";
  }
  render();
  mark();
}
async function deleteNote(nid) {
  const c = channel();
  if (allNotes().length === 1) return ask("삭제할 수 없음", "마지막 메모장은 삭제할 수 없습니다.");
  if (!(await ask("메모장 삭제", "이 메모장을 삭제할까요?"))) return;
  for(const g of c.categories)g.notes=g.notes.filter(x=>x.id!==nid);
  if (store.lastNoteId === nid) store.lastNoteId = allNotes()[0].id;
  render();
  mark();
  await save();
}
function toggleSelected(nid) {
  selectedNotes.has(nid) ? selectedNotes.delete(nid) : selectedNotes.add(nid);
  render();
}
$("#toggleNotes").onclick = () => { notesCollapsed = !notesCollapsed; render(); };
$("#bulkDelete").onclick = () => { selectMode = !selectMode; selectedNotes.clear(); notesCollapsed = false; render(); };
$("#cancelSelect").onclick = () => { selectMode = false; selectedNotes.clear(); render(); };
$("#deleteSelected").onclick = async () => {
  const c = channel();
  if (!selectedNotes.size) return ask("선택 필요", "삭제할 메모장을 선택하세요.");
  if (allNotes().length - selectedNotes.size < 1) return ask("삭제할 수 없음", "채널에는 메모장이 하나 이상 있어야 합니다.");
  if (!(await ask("여러 메모장 삭제", `선택한 ${selectedNotes.size}개 메모장을 삭제할까요?`))) return;
  for(const g of c.categories)g.notes=g.notes.filter(n=>!selectedNotes.has(n.id));
  if (selectedNotes.has(store.lastNoteId)) store.lastNoteId = allNotes()[0].id;
  selectedNotes.clear(); selectMode = false; render(); mark(); await save();
};
function syncListTitle(title) {
  const n = note();
  n.title = title.trim() || "제목 없음";
  if (n.titleLinked) {
    n.name = n.title;
    const b = $(`#notes [data-id="${n.id}"] b`);
    if (b) b.textContent = n.name;
  }
}
$("#title").oninput = () => {
  syncListTitle($("#title").value);
  capture();
  mark();
};
$("#editor").oninput = () => {
  capture();
  mark();
};
document
  .querySelectorAll("[data-cmd]")
  .forEach((b) => (b.onclick = () => document.execCommand(b.dataset.cmd)));
$("#font").onchange = (e) =>
  document.execCommand("fontName", false, e.target.value);
$("#textColor").onchange = (e) => {
  const selection = getSelection();
  if (!selection.rangeCount || selection.isCollapsed) return;
  const range = selection.getRangeAt(0), span = document.createElement("span");
  span.dataset.noteColor = e.target.value;
  span.append(range.extractContents());
  range.insertNode(span);
  selection.removeAllRanges();
  mark();
};
function fontSize(pt) {
  if (!getSelection().rangeCount) return;
  document.execCommand("fontSize", false, "7");
  $("#editor")
    .querySelectorAll('font[size="7"]')
    .forEach((x) => {
      x.removeAttribute("size");
      x.style.fontSize = pt + "pt";
    });
  mark();
}
$("#size").onchange = (e) =>
  fontSize(Math.max(6, Math.min(144, +e.target.value || 10)));
document.querySelectorAll("[data-size]").forEach(
  (b) =>
    (b.onclick = () => {
      $("#size").value = b.dataset.size;
      fontSize(+b.dataset.size);
    }),
);
$("#addChannel").onclick = async () => {
  const v = await ask("새 채널", "채널 이름을 입력하세요.", "새 채널");
  if (!v?.trim()) return;
  const g={id:id(),name:"메모장",notes:[newNote()]};
  const c = { id: id(), name: v.trim(), image: "", groupId:store.lastGroupId, categories: [g] };
  store.channels.push(c);
  store.lastChannelId = c.id;
  store.lastCategoryId = g.id;
  store.lastNoteId = g.notes[0].id;
  render();
  mark();
};
$("#addNote").onclick = async () => {
  const v=await ask("새 카테고리","카테고리 이름을 입력하세요.","새 카테고리");
  if(!v?.trim())return;
  const g={id:id(),name:v.trim(),notes:[]};channel().categories.push(g);store.lastCategoryId=g.id;render();mark();
};
async function addNote(gid) {
  await save();
  const n = newNote();
  const g=channel().categories.find(x=>x.id===gid)||category();
  g.notes.push(n);
  store.lastCategoryId=g.id;
  store.lastNoteId = n.id;
  render();
  mark();
}
async function insertImages(items) {
  $("#editor").focus();
  for (const x of items) {
    let src = x.dataUrl;
    if (!src && x instanceof File)
      src = await new Promise((r) => {
        const f = new FileReader();
        f.onload = () => r(f.result);
        f.readAsDataURL(x);
      });
    document.execCommand(
      "insertHTML",
      false,
      `<img src="${src}" style="width:100%;margin-left:auto;margin-right:auto"><p><br></p>`,
    );
  }
  mark();
}
$("#image").onclick = async () =>
  insertImages((await api().SelectImages()) || []);
$("#editor").onpaste = (e) => {
  const f = [...e.clipboardData.files].filter((x) =>
    x.type.startsWith("image/"),
  );
  if (f.length) {
    e.preventDefault();
    insertImages(f);
  }
};
$("#editor").ondragover = (e) => e.preventDefault();
$("#editor").ondrop = (e) => {
  const f = [...e.dataTransfer.files].filter((x) =>
    x.type.startsWith("image/"),
  );
  if (f.length) {
    e.preventDefault();
    insertImages(f);
  }
};
$("#editor").onclick = (e) => {
  selectedImage = e.target.tagName === "IMG" ? e.target : null;
  $("#editor")
    .querySelectorAll("img")
    .forEach((x) => x.classList.toggle("selected", x === selectedImage));
  imageTools();
};
function imageTools() {
  $("#imageTools").classList.toggle("show", !!selectedImage);
  $("#resizeBox").classList.toggle("show", !!selectedImage);
  if (selectedImage)
    $("#imageWidth").value = Math.round(
      parseFloat(selectedImage.style.width) || 100,
    );
  updateResizeBox();
}
function updateResizeBox() {
  if (!selectedImage) return;
  const r = selectedImage.getBoundingClientRect(), box = $("#resizeBox");
  box.style.left = r.left + "px"; box.style.top = r.top + "px";
  box.style.width = r.width + "px"; box.style.height = r.height + "px";
}
$("#editor").addEventListener("scroll", updateResizeBox);
addEventListener("resize", updateResizeBox);
document.querySelectorAll("#resizeBox i").forEach((handle) => {
  handle.onpointerdown = (e) => {
    if (!selectedImage) return;
    e.preventDefault(); handle.setPointerCapture(e.pointerId);
    const startX=e.clientX,startY=e.clientY,startW=selectedImage.offsetWidth,startH=selectedImage.offsetHeight,ratio=startW/startH,corner=handle.dataset.handle;
    const move=(p)=>{
      const dx=(corner.includes("w")?-1:1)*(p.clientX-startX),dy=(corner.includes("n")?-1:1)*(p.clientY-startY);
      let w=Math.max(40,startW+dx),h=Math.max(30,startH+dy);
      if(p.shiftKey){if(Math.abs(dx)>Math.abs(dy))h=w/ratio;else w=h*ratio}
      selectedImage.style.width=w+"px";selectedImage.style.height=h+"px";selectedImage.style.maxWidth="none";updateResizeBox();mark();
    };
    const up=()=>{handle.removeEventListener("pointermove",move);handle.removeEventListener("pointerup",up)};
    handle.addEventListener("pointermove",move);handle.addEventListener("pointerup",up);
  };
});
$("#imageWidth").oninput = (e) => {
  if (selectedImage) {
    selectedImage.style.width =
      Math.max(5, Math.min(100, +e.target.value)) + "%";
    mark();
  }
};
document.querySelectorAll("[data-align]").forEach(
  (b) =>
    (b.onclick = () => {
      if (!selectedImage) return;
      const a = b.dataset.align;
      selectedImage.style.marginLeft = a === "left" ? "0" : "auto";
      selectedImage.style.marginRight = a === "right" ? "0" : "auto";
      selectedImage.style.display = "block";
      mark();
    }),
);
let cropResolve=null,cropState={zoom:1,x:0,y:0};
function updateCrop(){const img=$("#cropImage");img.style.transform=`translate(calc(-50% + ${cropState.x}px),calc(-50% + ${cropState.y}px)) scale(${cropState.zoom})`}
function resetCrop(){const img=$("#cropImage"),stage=$(".crop-stage"),size=stage.clientWidth||320;if(img.naturalWidth&&img.naturalHeight){const base=Math.max(size/img.naturalWidth,size/img.naturalHeight);img.style.width=img.naturalWidth*base+"px";img.style.height=img.naturalHeight*base+"px"}cropState={zoom:1,x:0,y:0};$("#cropZoom").value=100;updateCrop()}
function editChannelImage(src){const img=$("#cropImage");img.onload=resetCrop;img.src=src;$("#imageEditModal").classList.add("show");if(img.complete)resetCrop();return new Promise(r=>cropResolve=r)}
function closeCrop(value){$("#imageEditModal").classList.remove("show");cropResolve?.(value);cropResolve=null}
$("#cropZoom").oninput=e=>{cropState.zoom=+e.target.value/100;updateCrop()};$("#cropReset").onclick=resetCrop;$("#cropCancel").onclick=()=>closeCrop(null);$("#closeImageEdit").onclick=()=>closeCrop(null);
$(".crop-stage").onpointerdown=e=>{e.preventDefault();const stage=e.currentTarget,pid=e.pointerId,sx=e.clientX,sy=e.clientY,ox=cropState.x,oy=cropState.y;stage.setPointerCapture?.(pid);const move=p=>{if(p.pointerId!==pid)return;cropState.x=ox+p.clientX-sx;cropState.y=oy+p.clientY-sy;updateCrop()};const stop=p=>{if(p.pointerId!==undefined&&p.pointerId!==pid)return;window.removeEventListener('pointermove',move);window.removeEventListener('pointerup',stop);window.removeEventListener('pointercancel',stop);stage.removeEventListener('lostpointercapture',stop);if(stage.hasPointerCapture?.(pid))stage.releasePointerCapture(pid)};window.addEventListener('pointermove',move);window.addEventListener('pointerup',stop);window.addEventListener('pointercancel',stop);stage.addEventListener('lostpointercapture',stop)};
$("#cropApply").onclick=()=>{const img=$("#cropImage"),canvas=document.createElement('canvas');canvas.width=canvas.height=200;const ctx=canvas.getContext('2d'),scale=Math.max(200/img.naturalWidth,200/img.naturalHeight)*cropState.zoom,w=img.naturalWidth*scale,h=img.naturalHeight*scale,f=200/($(".crop-stage").clientWidth||320);ctx.drawImage(img,(200-w)/2+cropState.x*f,(200-h)/2+cropState.y*f,w,h);closeCrop(canvas.toDataURL('image/png'))};
$("#openSettings").onclick = () => $("#modal").classList.add("show");
$("#closeSettings").onclick = () => $("#modal").classList.remove("show");
$("#modal").onclick = (e) => {
  if (e.target.id === "modal") e.currentTarget.classList.remove("show");
};
$("#theme").onchange = (e) => {
  store.theme = e.target.value;
  applyTheme();
  mark();
};
$("#showGroupPopup").onchange=(e)=>{store.showGroupPopup=e.target.checked;store.settingsVersion=1;mark()};
async function showStartupGroups(){
  const locked=new Set(await api().LockedGroups()),box=$("#startupGroups");
  box.innerHTML=store.groups.map(g=>`<button data-id="${g.id}" ${locked.has(g.id)?'disabled':''}><b>${esc(g.name)}</b><small>${locked.has(g.id)?'다른 창에서 작업 중':'선택하여 편집 시작'}</small></button>`).join('')+'<button id="startupAddGroup" class="startup-add"><b>＋ 새 그룹 생성</b><small>새 작업 공간을 만들어 시작합니다</small></button>';
  box.querySelectorAll('button[data-id]').forEach(b=>b.onclick=()=>selectGroup(b.dataset.id));
  $("#startupAddGroup").onclick=addGroup;
  $("#startupGroupModal").classList.add("show");
}
$("#backup").onclick = async () => {
  await save();
  const p = await api().BackupAll();
  if (p) $("#status").textContent = "백업 완료 · " + p;
};
$("#backupGroup").onclick=async()=>{await save();const gid=$("#groupDataSelect").value,p=await api().BackupGroup(gid);if(p)$("#status").textContent="그룹 백업 완료 · "+p};
$("#restoreGroup").onclick=async()=>{const bundle=await api().RestoreGroup();if(!bundle?.group?.id)return;await save();store.groups.push(bundle.group);store.channels.push(...(bundle.channels||[]));render();mark();await selectGroup(bundle.group.id);$("#modal").classList.remove("show");$("#status").textContent="그룹 복원 완료"};
$("#restore").onclick = async () => {
  if (!(await ask("전체 복원", "현재 데이터를 백업 파일로 교체할까요?"))) return;
  const s = await api().RestoreAll();
  if (s?.channels) {
    store = s;
    dirty = false;
    render();
    $("#modal").classList.remove("show");
  }
};
$("#export").onclick = async () => {
  capture();
  const n = note(),
    p = await api().ExportNote(n.title, n.content);
  if (p) $("#status").textContent = "내보내기 완료 · " + p;
};
async function manualSave(){capture();dirty=true;clearTimeout(saveTimer);await save();$("#status").textContent=$("#status").textContent.replace("자동저장 완료","수동 저장 완료")}
$("#manualSave").onclick=manualSave;
document.addEventListener("keydown",e=>{if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==="s"){e.preventDefault();manualSave()}});
$("#import").onclick = async () => {
  const imported = await api().ImportNote();
  if (!imported?.id) return;
  await save();
  category().notes.push(imported);
  store.lastNoteId = imported.id;
  render();
  mark();
  $("#status").textContent = "메모 불러오기 완료";
};
async function boot() {
  try {
    store = await api().GetStore();
    const fonts = await api().SystemFonts();
    $("#font").innerHTML = fonts
      .map((f) => `<option value="${esc(f)}">${esc(f)}</option>`)
      .join("");
    normalize();render();
    if(!closeHandlerRegistered&&window.runtime?.EventsOn){closeHandlerRegistered=true;window.runtime.EventsOn("app:before-close",async()=>{capture();dirty=true;clearTimeout(saveTimer);try{const t=await api().SaveStore(store);dirty=false;$("#status").textContent=`${t} · 종료 전 저장 완료`;await api().FinishClose()}catch(e){$("#status").textContent="종료 전 저장 실패: "+e}})}
    if(store.showGroupPopup!==false) await showStartupGroups();
    else { try{await api().AcquireGroup(store.lastGroupId)}catch{await showStartupGroups()} }
  } catch {
    setTimeout(boot, 100);
  }
}
boot();
