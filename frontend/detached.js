import "./detached.css";
import { Call, Events } from "/wails/runtime.js";

const api=new Proxy({}, {get:(_,method)=>(...args)=>Call.ByName(`main.App.${String(method)}`,...args)});
const noteID=new URLSearchParams(location.search).get("note")||"";
const $=selector=>document.querySelector(selector);
let saveTimer=null,savedRange=null,loaded=false,selectedImage=null,imageInsertWidth=100;

$("#detachedApp").innerHTML=`<div class="detached-shell"><header><input id="detachedTitle"><button id="detachedSave" title="Ctrl+S">저장</button><button id="reattach" title="메인 창으로 합치기">↙ 메인 창으로</button></header><div class="detached-toolbar"><button data-cmd="bold"><b>B</b></button><button data-cmd="italic"><i>I</i></button><button data-cmd="underline"><u>U</u></button><select id="detachedColor"><option value="default">기본색</option><option value="red">빨강</option><option value="orange">주황</option><option value="green">초록</option><option value="blue">파랑</option><option value="purple">보라</option></select><select id="detachedFont"></select><label><input id="detachedSize" type="number" min="6" max="144" value="10"> pt</label><div class="detached-presets">${[8,9,10,11,12,14,16,18,20,24].map(size=>`<button data-size="${size}">${size}</button>`).join("")}</div><button id="detachedImage">이미지 삽입</button><div id="detachedImageTools"><input id="detachedImageWidth" type="number" min="5" max="100" value="100"><span>%</span><button data-align="left">왼쪽</button><button data-align="center">가운데</button><button data-align="right">오른쪽</button></div></div><div class="detached-editor-wrap"><div id="detachedEditor" contenteditable="true" spellcheck="true"></div><div id="detachedResizeBox"><i data-handle="nw"></i><i data-handle="ne"></i><i data-handle="sw"></i><i data-handle="se"></i></div></div><footer id="detachedStatus">불러오는 중…</footer></div>`;
$("#detachedEditor").setAttribute("data-file-drop-target", "");

function rememberSelection(){const s=getSelection();if(!s.rangeCount)return;const r=s.getRangeAt(0),n=r.commonAncestorContainer;if($("#detachedEditor").contains(n.nodeType===Node.ELEMENT_NODE?n:n.parentElement))savedRange=r.cloneRange()}
function showSavedSelection(){if(savedRange&&globalThis.Highlight&&globalThis.CSS?.highlights)CSS.highlights.set("saved-editor-selection",new Highlight(savedRange.cloneRange()))}
function hideSavedSelection(){globalThis.CSS?.highlights?.delete("saved-editor-selection")}
function restoreSelection(){if(!savedRange)return;const s=getSelection();s.removeAllRanges();s.addRange(savedRange.cloneRange())}
document.addEventListener("selectionchange",rememberSelection);
function mark(){if(!loaded)return;clearTimeout(saveTimer);saveTimer=setTimeout(save,700)}
async function save(){if(!loaded)return;clearTimeout(saveTimer);try{const time=await api.SaveDetachedNote(noteID,$("#detachedTitle").value,$("#detachedEditor").innerHTML);$("#detachedStatus").textContent=`${time} · 자동저장 완료`}catch(e){$("#detachedStatus").textContent="저장 실패: "+e}}
function applySize(){const preserved=savedRange?.cloneRange();if(!preserved)return;$("#detachedEditor").focus({preventScroll:true});const s=getSelection();s.removeAllRanges();s.addRange(preserved);document.execCommand("fontSize",false,"7");$("#detachedEditor").querySelectorAll('font[size="7"]').forEach(el=>{el.removeAttribute("size");el.style.fontSize=Math.max(6,Math.min(144,+$("#detachedSize").value||10))+"pt"});rememberSelection();showSavedSelection();mark()}
document.querySelectorAll("[data-cmd]").forEach(button=>{button.onmousedown=e=>e.preventDefault();button.onclick=()=>{restoreSelection();document.execCommand(button.dataset.cmd);rememberSelection();mark()}});
$("#detachedFont").onchange=e=>{restoreSelection();document.execCommand("fontName",false,e.target.value);rememberSelection();mark()};
$("#detachedSize").onpointerdown=()=>{rememberSelection();showSavedSelection()};
$("#detachedSize").onfocus=showSavedSelection;
$("#detachedSize").onchange=applySize;
$("#detachedSize").onkeydown=e=>{if(e.key==="Enter"){e.preventDefault();applySize()}};
document.querySelectorAll("[data-size]").forEach(button=>{button.onmousedown=e=>e.preventDefault();button.onclick=()=>{$("#detachedSize").value=button.dataset.size;applySize()}});
$("#detachedEditor").addEventListener("pointerdown",hideSavedSelection);
$("#detachedColor").onchange=e=>{restoreSelection();const s=getSelection();if(!s.rangeCount||s.isCollapsed)return;const r=s.getRangeAt(0),span=document.createElement("span");span.dataset.noteColor=e.target.value;span.append(r.extractContents());r.insertNode(span);rememberSelection();mark()};
$("#detachedTitle").oninput=mark;$("#detachedEditor").oninput=mark;
async function insertImages(items){$("#detachedEditor").focus();restoreSelection();for(const item of items){let src=item.dataUrl;if(!src&&item instanceof File)src=await new Promise(resolve=>{const reader=new FileReader();reader.onload=()=>resolve(reader.result);reader.readAsDataURL(item)});document.execCommand("insertHTML",false,`<img src="${src}" style="width:${imageInsertWidth}%;margin-left:auto;margin-right:auto"><p><br></p>`)}mark()}
function placeDropCaret(x,y){const r=document.caretRangeFromPoint?.(x,y);if(!r||!$("#detachedEditor").contains(r.startContainer))return;const s=getSelection();s.removeAllRanges();s.addRange(r);savedRange=r.cloneRange()}
$("#detachedImage").onclick=async()=>insertImages(await api.SelectImages()||[]);
$("#detachedEditor").onpaste=e=>{const files=[...e.clipboardData.files].filter(f=>f.type.startsWith("image/"));if(files.length){e.preventDefault();insertImages(files)}};
$("#detachedEditor").ondragover=e=>e.preventDefault();$("#detachedEditor").ondrop=e=>{e.preventDefault();if(window._wails?.flags?.enableFileDrop)return;const files=[...e.dataTransfer.files].filter(f=>f.type.startsWith("image/"));if(files.length)insertImages(files)};
function updateImageTools(){
  $("#detachedImageTools").classList.toggle("show",!!selectedImage);$("#detachedResizeBox").classList.toggle("show",!!selectedImage);
  if(selectedImage)$("#detachedImageWidth").value=Math.round(parseFloat(selectedImage.style.width)||100);
  updateResizeBox();
}
function updateResizeBox(){if(!selectedImage)return;const r=selectedImage.getBoundingClientRect(),box=$("#detachedResizeBox");box.style.left=r.left+"px";box.style.top=r.top+"px";box.style.width=r.width+"px";box.style.height=r.height+"px"}
$("#detachedEditor").addEventListener("click",e=>{selectedImage=e.target.tagName==="IMG"?e.target:null;$("#detachedEditor").querySelectorAll("img").forEach(img=>img.classList.toggle("selected",img===selectedImage));updateImageTools()});
$("#detachedEditor").addEventListener("scroll",updateResizeBox);addEventListener("resize",updateResizeBox);
document.querySelectorAll("#detachedResizeBox i").forEach(handle=>{handle.onpointerdown=e=>{if(!selectedImage)return;e.preventDefault();const image=selectedImage,pid=e.pointerId,startX=e.clientX,startY=e.clientY,startW=image.offsetWidth,startH=image.offsetHeight,ratio=startW/startH,corner=handle.dataset.handle;handle.setPointerCapture?.(pid);const move=p=>{if(p.pointerId!==pid)return;const dx=(corner.includes("w")?-1:1)*(p.clientX-startX),dy=(corner.includes("n")?-1:1)*(p.clientY-startY);let width=Math.max(40,startW+dx),height=Math.max(30,startH+dy);if(p.shiftKey){if(Math.abs(dx)>Math.abs(dy))height=width/ratio;else width=height*ratio}image.style.width=width+"px";image.style.height=height+"px";image.style.maxWidth="none";updateResizeBox();mark()};const stop=p=>{if(p.pointerId!==undefined&&p.pointerId!==pid)return;removeEventListener("pointermove",move);removeEventListener("pointerup",stop);removeEventListener("pointercancel",stop);if(handle.hasPointerCapture?.(pid))handle.releasePointerCapture(pid)};addEventListener("pointermove",move);addEventListener("pointerup",stop);addEventListener("pointercancel",stop)}});
$("#detachedImageWidth").oninput=e=>{if(!selectedImage)return;imageInsertWidth=Math.max(5,Math.min(100,+e.target.value||100));selectedImage.style.width=imageInsertWidth+"%";selectedImage.style.height="auto";selectedImage.style.maxWidth="100%";updateResizeBox();mark()};
$("#detachedImageWidth").onchange=()=>api.SetImageInsertWidth(imageInsertWidth);
document.querySelectorAll("#detachedImageTools [data-align]").forEach(button=>button.onclick=()=>{if(!selectedImage)return;const align=button.dataset.align;selectedImage.style.marginLeft=align==="left"?"0":"auto";selectedImage.style.marginRight=align==="right"?"0":"auto";selectedImage.style.display="block";updateResizeBox();mark()});
$("#detachedSave").onclick=async()=>{await save();$("#detachedStatus").textContent=$("#detachedStatus").textContent.replace("자동저장 완료","수동 저장 완료")};
addEventListener("keydown",e=>{if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==="s"){e.preventDefault();$("#detachedSave").click()}});
$("#reattach").onclick=async()=>{await save();await api.ReattachDetachedNote(noteID)};
addEventListener("beforeunload",save);
Events.On("note:updated",event=>{const data=event.data||{};if(data.noteId!==noteID||document.activeElement===$("#detachedEditor")||document.activeElement===$("#detachedTitle"))return;$("#detachedTitle").value=data.title;$("#detachedEditor").innerHTML=data.content});
Events.On("editor:images-dropped",event=>{const data=event.data||{};if(data.target!==noteID)return;placeDropCaret(data.x,data.y);insertImages(data.images||[])});
Events.On("image:insert-width",event=>{const width=+event.data;if(width>=5&&width<=100)imageInsertWidth=width});

async function boot(){try{const data=await api.GetDetachedNote(noteID),fonts=await api.SystemFonts();document.documentElement.dataset.theme=data.theme||"dark";imageInsertWidth=Math.max(5,Math.min(100,+data.imageInsertWidth||100));$("#detachedTitle").value=data.note.title||"";$("#detachedEditor").innerHTML=data.note.content||"<p></p>";$("#detachedFont").innerHTML=fonts.map(f=>`<option value="${f.replaceAll('"','&quot;')}">${f}</option>`).join("");loaded=true;$("#detachedStatus").textContent="분리된 메모 · 메인 창 상단으로 창을 끌면 다시 합쳐집니다"}catch(e){$("#detachedStatus").textContent="메모 로딩 실패: "+e}}
boot();
