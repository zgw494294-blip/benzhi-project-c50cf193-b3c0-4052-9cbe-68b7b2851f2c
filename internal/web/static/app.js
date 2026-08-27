const state={batches:[],selected:null};
const $=selector=>document.querySelector(selector);
const escapeHTML=value=>{const node=document.createElement('span');node.textContent=String(value??'');return node.innerHTML};
const idempotency=prefix=>`${prefix}-${Date.now()}-${crypto.randomUUID()}`;
const number=(form,name)=>Number(form.get(name));

async function api(path,options={}){
  const response=await fetch(path,{...options,headers:{'Content-Type':'application/json',...(options.headers||{})}});
  const body=await response.json().catch(()=>({error:{message:'服务返回了无法解析的响应'}}));
  if(!response.ok)throw new Error(body.error?.message||`请求失败 (${response.status})`);
  return body.data;
}
function notice(message){const box=$('#notice');box.textContent=message;box.classList.add('show');setTimeout(()=>box.classList.remove('show'),3200)}
async function digest(text){const data=new TextEncoder().encode(text.trim());const hash=await crypto.subtle.digest('SHA-256',data);return [...new Uint8Array(hash)].map(byte=>byte.toString(16).padStart(2,'0')).join('')}

async function loadBatches(){
  const filter=$('#batch-filter');
  const params=filter?new URLSearchParams(new FormData(filter)):new URLSearchParams();
  [...params].forEach(([key,value])=>{if(!value)params.delete(key)});
  const filtered=params.toString();
  const result=await api(`/api/batches${filtered?`?${filtered}`:''}`);
  state.batches=Array.isArray(result)?result:(result.batches||[]);
  if(!Array.isArray(result)){
    const counts=result.status_counts||{};
    $('#batch-stats').textContent=`共 ${result.total} 个批次 · 待整改 ${counts.REMEDIATION_REQUIRED||0} · 待复核 ${counts.READY_REVIEW||0} · 已冻结 ${counts.FROZEN||0}`;
  }
  $('#batch-list').innerHTML=state.batches.length?state.batches.map(record=>`<button class="batch" data-id="${escapeHTML(record.batch.batch_id)}"><strong>${escapeHTML(record.batch.batch_id)}</strong><span>${escapeHTML(record.batch.species_name)} · ${escapeHTML(record.batch.source_region)}</span><span class="meta">版本 ${record.batch.version} · 样本 ${record.batch.sample_count} 份</span><span class="badge">${escapeHTML(record.batch.status)}</span></button>`).join(''):'<p>尚无批次，请先建立档案。</p>';
  document.querySelectorAll('.batch').forEach(button=>button.addEventListener('click',()=>selectBatch(button.dataset.id)));
  if(state.selected){const current=state.batches.find(item=>item.batch.batch_id===state.selected.batch.batch_id);if(current)renderSelected(current)}
}
async function selectBatch(id){const record=await api(`/api/batches/${encodeURIComponent(id)}`);renderSelected(record);location.hash='operate'}
function renderSelected(record){
  state.selected=record;$('#operate').classList.remove('hidden');
  $('#selection').innerHTML=`<strong>${escapeHTML(record.batch.batch_id)}</strong> · ${escapeHTML(record.batch.status)} · 版本 ${record.batch.version}`;
  renderQuality(record.quality);renderAudit(record);prefillRemediation(record);loadPreview(record.batch.batch_id);
  if(record.batch.status==='FROZEN'&&record.reviews.length){const review=record.reviews.at(-1);api(`/api/credentials/${encodeURIComponent(review.credential_id)}`).then(renderCredential).catch(()=>renderCredential({credential_id:review.credential_id,snapshot_digest:review.snapshot_digest,batch_id:record.batch.batch_id,verify_text:`SEEDVAULT:${review.credential_id}:${review.snapshot_digest}`}))}
}
function renderQuality(quality){
  if(!quality||!quality.checks){$('#quality-result').innerHTML='';return}
  const issues=quality.issues||[];
  $('#quality-result').innerHTML=`<div class="result ${quality.passed?'':'fail'}"><strong>${quality.passed?'质量规则通过，可提交复核':'检测被阻断，需要整改复测'}</strong><ul>${quality.checks.map(check=>`<li>${check.passed?'✓':'✕'} <code>${escapeHTML(check.code)}</code> ${escapeHTML(check.message)}</li>`).join('')}</ul>${issues.length?`<small>问题代码：${issues.map(issue=>escapeHTML(issue.code)).join('、')}</small>`:''}</div>`;
}
function renderAudit(record){
  $('#manifest').innerHTML=record.manifest?.length?`<h3>冻结证据清单</h3><ul>${record.manifest.map(item=>`<li><strong>${escapeHTML(item.kind)} / ${escapeHTML(item.id)}</strong> — ${escapeHTML(item.summary)}<br><small>${escapeHTML(item.digest)}</small></li>`).join('')}</ul>`:'<p>冻结后将在此显示证据清单。</p>';
  $('#timeline').innerHTML=(record.timeline||[]).map(item=>`<article><strong>${escapeHTML(item.summary)}</strong><div>${escapeHTML(item.actor)} · ${new Date(item.occurred_at).toLocaleString()}</div><small>序号 ${item.sequence} · ${escapeHTML(item.hash.slice(0,16))}…</small></article>`).join('');
}
async function loadPreview(batchID){
  try{
    const preview=await api(`/api/batches/${encodeURIComponent(batchID)}/evidence-preview`);
    if(!state.selected||state.selected.batch.batch_id!==batchID)return;
    $('#preview').innerHTML=`<h3>冻结前证据预览</h3><p>清单摘要 <code>${escapeHTML(preview.manifest_digest)}</code> · ${preview.blocked?'当前阻断':'可以进入复核'}</p>${(preview.comparisons||[]).map(item=>`<article><strong>${escapeHTML(item.original_test_id)} → ${escapeHTML(item.retest_id)}</strong><div>活力 ${item.germination_delta.toFixed(2)} · 纯度 ${item.purity_delta.toFixed(2)} · 含水率 ${item.moisture_delta.toFixed(2)}</div><small>${item.regression_codes?.length?escapeHTML(item.regression_codes.join('、')):'无回归问题'}</small></article>`).join('')}`;
  }catch(error){$('#preview').textContent=error.message}
}
function prefillRemediation(record){
  const form=$('#remediation-form');if(!record.tests.length)return;
  const last=record.tests.at(-1);form.elements.original_test_id.value=last.test_id;
  form.elements.retest_id.value=`${last.test_id}-R${record.remediations.length+1}`;
  form.elements.issue_codes.value=(record.quality.issues||[]).map(issue=>issue.code).join(',');
}
function requireSelected(){if(!state.selected){notice('请先选择批次');return false}return true}

$('#batch-form').addEventListener('submit',async event=>{
  event.preventDefault();const form=new FormData(event.currentTarget);const payload=Object.fromEntries(form);payload.sample_count=number(form,'sample_count');payload.role='receiver';payload.idempotency_key=idempotency('create');
  try{const record=await api('/api/batches',{method:'POST',body:JSON.stringify(payload)});notice('批次档案已建立');await loadBatches();renderSelected(record)}catch(error){notice(error.message)}
});
$('#test-form').addEventListener('submit',async event=>{
  event.preventDefault();if(!requireSelected())return;const form=new FormData(event.currentTarget);
  const payload={test_id:form.get('test_id'),expected_version:state.selected.batch.version,actor:form.get('actor'),role:'tester',idempotency_key:idempotency('test'),test:{method_code:form.get('method_code'),replicates:number(form,'replicates'),germination_rate:number(form,'germination_rate'),purity_rate:number(form,'purity_rate'),moisture_rate:number(form,'moisture_rate'),contamination_flag:form.get('contamination_flag')==='on',contamination_note:form.get('contamination_note'),evidence_digest:await digest(form.get('evidence_text'))}};
  try{const record=await api(`/api/batches/${encodeURIComponent(state.selected.batch.batch_id)}/tests`,{method:'POST',body:JSON.stringify(payload)});renderSelected(record);await loadBatches();notice('检测已保存并完成规则检查')}catch(error){notice(error.message)}
});
$('#remediation-form').addEventListener('submit',async event=>{
  event.preventDefault();if(!requireSelected())return;const form=new FormData(event.currentTarget);
  const payload={original_test_id:form.get('original_test_id'),retest_id:form.get('retest_id'),issue_codes:String(form.get('issue_codes')).split(',').map(v=>v.trim()).filter(Boolean),explanation:form.get('explanation'),expected_version:state.selected.batch.version,actor:form.get('actor'),role:'receiver',idempotency_key:idempotency('remediation'),retest:{method_code:form.get('method_code'),replicates:number(form,'replicates'),germination_rate:number(form,'germination_rate'),purity_rate:number(form,'purity_rate'),moisture_rate:number(form,'moisture_rate'),contamination_flag:false,evidence_digest:await digest(form.get('evidence_text'))}};
  try{const record=await api(`/api/batches/${encodeURIComponent(state.selected.batch.batch_id)}/remediations`,{method:'POST',body:JSON.stringify(payload)});renderSelected(record);await loadBatches();notice(record.quality.passed?'整改复测通过':'复测仍未通过，流程继续阻断')}catch(error){notice(error.message)}
});
$('#review-form').addEventListener('submit',async event=>{
  event.preventDefault();if(!requireSelected())return;const form=new FormData(event.currentTarget);
  const payload={decision:form.get('decision'),issue_refs:String(form.get('issue_refs')).split(',').map(v=>v.trim()).filter(Boolean),comment:form.get('comment'),expected_version:state.selected.batch.version,actor:form.get('actor'),role:'reviewer',idempotency_key:idempotency('review')};
  try{const record=await api(`/api/batches/${encodeURIComponent(state.selected.batch.batch_id)}/reviews`,{method:'POST',body:JSON.stringify(payload)});renderSelected(record);await loadBatches();notice(payload.decision==='APPROVE'?'复核已通过，可由管理员冻结':'批次已退回补充')}catch(error){notice(error.message)}
});
$('#freeze').addEventListener('click',async()=>{
  if(!requireSelected())return;const payload={expected_version:state.selected.batch.version,actor:$('#admin').value,role:'administrator',idempotency_key:idempotency('freeze')};
  if(!confirm('冻结后批次不可再编辑，确定签发入库凭据吗？'))return;
  try{const result=await api(`/api/batches/${encodeURIComponent(state.selected.batch.batch_id)}/freeze`,{method:'POST',body:JSON.stringify(payload)});renderSelected(result.batch);renderCredential(result.credential);await loadBatches();notice('核验快照已冻结，凭据已签发')}catch(error){notice(error.message)}
});
function renderCredential(value){$('#credential-result').innerHTML=`<div class="credential"><strong>入库凭据 ${escapeHTML(value.credential_id)}</strong><p>批次 ${escapeHTML(value.batch_id)} · 状态 ${escapeHTML(value.status||'VALID')}</p><small>冻结快照摘要</small><code>${escapeHTML(value.snapshot_digest)}</code><small>二维码文本</small><code>${escapeHTML(value.verify_text||'')}</code><a href="/verify">前往公开验证页 →</a>${value.status!=='REVOKED'?`<form id="revoke-form" class="form-grid compact"><label>撤销原因<input name="reason" required></label><label>管理员<input name="actor" value="管理员-01" required></label><button class="danger" type="submit">撤销凭据</button></form>`:`<small>撤销原因：${escapeHTML(value.revocation_reason||'')}</small>`}</div>`}

document.addEventListener('submit',async event=>{
  if(event.target.id!=='revoke-form')return;
  event.preventDefault();
  const form=new FormData(event.target), credential=state.selected?.reviews?.at(-1)?.credential_id;
  if(!credential)return;
  try{const value=await api(`/api/credentials/${encodeURIComponent(credential)}/revoke`,{method:'POST',body:JSON.stringify({reason:form.get('reason'),actor:form.get('actor'),role:'administrator',idempotency_key:idempotency('revoke')})});renderCredential(value);await loadBatches();await loadPreview(state.selected.batch.batch_id);notice('凭据已撤销')}catch(error){notice(error.message)}
});

document.querySelectorAll('.tab').forEach(button=>button.addEventListener('click',()=>{document.querySelectorAll('.tab').forEach(item=>item.classList.remove('active'));document.querySelectorAll('.tab-panel').forEach(item=>item.classList.add('hidden'));button.classList.add('active');$(`#tab-${button.dataset.tab}`).classList.remove('hidden')}));
$('#refresh').addEventListener('click',()=>loadBatches().catch(error=>notice(error.message)));
$('#batch-filter').addEventListener('submit',event=>{event.preventDefault();loadBatches().catch(error=>notice(error.message))});
const today=new Date();today.setMinutes(today.getMinutes()-today.getTimezoneOffset());$('#batch-form').elements.harvest_date.value=today.toISOString().slice(0,10);
loadBatches().catch(error=>notice(error.message));
