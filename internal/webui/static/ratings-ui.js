/* Show provider age classification in the same metadata line as year/runtime. */
(function(){
  if(typeof metaHTML!=='function')return;
  const base=metaHTML;
  metaHTML=function(item,detail=false){
    let html=base(item,detail);
    if(!detail||!item?.content_rating)return html;
    const label=String(item.content_rating).trim();
    if(!label)return html;
    const badge=`<span class="content-rating-badge">${escapeHTML(label.toUpperCase()==='L'?'L':label)}</span>`;
    const direct='<span class="direct-badge small">DIRECT PLAY</span>';
    return html.includes(direct)?html.replace(direct,badge+direct):html+badge;
  };
})();
