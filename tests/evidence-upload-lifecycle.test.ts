import assert from "node:assert/strict";
import { test } from "node:test";
import {
  uploadEvidence, completeEvidence, listEvidenceChecks,
  EvidenceCompletionPending, EvidenceRequestError, MAX_EVIDENCE_BYTES,
  type CompletionTicket,
} from "../apps/evt-ui-expo/src/api/evidence";

const scope = { caseId: "assigned-case", checkId: "employment.company_and_dates" };
const file = { uri: "file:///private-device/file.pdf", name: "file.pdf", size: 1, mimeType: "application/pdf" };
const session = "10000000-0000-4000-8000-000000000001";
const upload = "20000000-0000-4000-8000-000000000002";
const signed = "https://private-storage.invalid/signed?PRIVATE_URL_SIGNATURE=secret";
const headers = { "Content-Type": "application/pdf", "x-amz-acl": "private" };
const initialized = { ...scope, uploadSessionId: session, expiresAt: "2030-01-01T00:00:00Z", uploads: [{ uploadId: upload, size: 1, mimeType: "application/pdf", method: "PUT", url: signed, headers, storageKey: "PRIVATE_STORAGE_KEY" }] };
const completed = { ...scope, evidence: [{ uploadId: upload, status: "RECEIVED", scanStatus: "PENDING", evidence_hash: "a".repeat(64) }] };
const ticket: CompletionTicket = { ...scope, uploadSessionId: session, uploadId: upload, name: file.name, size: file.size, mimeType: file.mimeType, expiresAt: initialized.expiresAt };
const base = "https://api.example.invalid";
const token = "PRIVATE_ACCESS_TOKEN";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

test("authorized init -> exact PUT headers -> completion, with no client hash or identity assertion", async () => {
  const steps: string[] = [];
  const transport = (async (url, init) => {
    const request = JSON.parse(init!.body as string);
    assert.equal(new Headers(init!.headers).get("Authorization"), `Bearer ${token}`);
    assert.equal(JSON.stringify(request).includes("sha256"), false);
    assert.equal(JSON.stringify(request).includes(file.uri), false);
    assert.equal(JSON.stringify(request).includes("principal"), false);
    if (String(url).endsWith("evidence:init")) {
      steps.push("init"); assert.deepEqual(request, { ...scope, files: [{ name: file.name, size: 1, mimeType: file.mimeType }] }); return json(initialized);
    }
    steps.push("complete"); assert.deepEqual(request, { uploadSessionId: session, uploads: [{ uploadId: upload, size: 1, mimeType: file.mimeType }] });return json(completed);
  }) as typeof fetch;
  const receipt = await uploadEvidence({ apiBase: base, accessToken: token, scope, file, transport,
    put: async (url, uri, receivedHeaders) => { steps.push("PUT"); assert.equal(url, signed);assert.equal(uri,file.uri);assert.deepEqual(receivedHeaders, headers);assert.equal("Authorization" in receivedHeaders,false);return {status:200}; },
  });
  assert.deepEqual(steps,["init","PUT","complete"]);
  assert.equal(receipt.status,"RECEIVED");assert.equal(receipt.scanStatus,"PENDING");assert.equal(receipt.evidenceHash,"a".repeat(64));
  assert.equal(JSON.stringify(receipt).includes(signed),false);assert.equal(JSON.stringify(receipt).includes("PRIVATE_STORAGE_KEY"),false);
});

test("uncertain completion retains only a resumable ticket and retries without another init or PUT", async () => {
 let inits=0,puts=0,completes=0;
 const transport=(async(url)=>{
  if(String(url).endsWith("evidence:init")){inits++;return json(initialized)}
  completes++;if(completes===1)throw new Error("PRIVATE_ERROR_CONTENTS "+signed);return json(completed);
 }) as typeof fetch;
 let pending: CompletionTicket | undefined;
 try{await uploadEvidence({apiBase:base,accessToken:token,scope,file,transport,put:async()=>{puts++;return{status:204}}});assert.fail("completion falsely succeeded")}
 catch(error){assert.ok(error instanceof EvidenceCompletionPending);pending=error.ticket;assert.equal(JSON.stringify(error).includes("PRIVATE_ERROR_CONTENTS"),false);assert.equal(JSON.stringify(error).includes(signed),false);assert.equal(JSON.stringify(error).includes(token),false)}
 const receipt=await completeEvidence(base,token,pending!,transport);
 assert.equal(receipt.status,"RECEIVED");assert.deepEqual({inits,puts,completes},{inits:1,puts:1,completes:2});
});

test("authorization and permanent expiry failures are not represented as received or retryable success",async()=>{
 for(const [status,code] of [[401,"unauthorized"],[404,"not_found"],[409,"upload_session_expired"],[409,"duplicate_evidence"]] as const){
  await assert.rejects(()=>completeEvidence(base,token,ticket,(async()=>json({error:code},status)) as typeof fetch),(error:unknown)=>error instanceof EvidenceRequestError && error.code===code && !(error instanceof EvidenceCompletionPending));
 }
});

test("malformed receipt, unexpected scope and a false clean/verified response never mark evidence accepted",async()=>{
 for(const data of [{},{...completed,caseId:"foreign-case"},{...completed,evidence:[]},{...completed,evidence:[{...completed.evidence[0],scanStatus:"CLEAN"}]},{...completed,evidence:[{...completed.evidence[0],evidence_hash:"A".repeat(64)}]}]){
  await assert.rejects(()=>completeEvidence(base,token,ticket,(async()=>json(data)) as typeof fetch),EvidenceCompletionPending);
 }
});

test("PUT failure never invokes completion or exposes private storage errors",async()=>{
 let requests=0;
 await assert.rejects(()=>uploadEvidence({apiBase:base,accessToken:token,scope,file,transport:(async()=>{requests++;return json(initialized)}) as typeof fetch,put:async()=>{throw new Error(signed+token)}}),(error:unknown)=>error instanceof EvidenceRequestError && !JSON.stringify(error).includes(token) && !JSON.stringify(error).includes(signed));
 assert.equal(requests,1);
});

test("scope discovery returns authoritative capabilities only; empty assignments remain empty",async()=>{
 const rows=[{...scope,canUpload:true,canReview:false}];
 assert.deepEqual(await listEvidenceChecks(base,token,(async()=>json({items:rows})) as typeof fetch),rows);
 assert.deepEqual(await listEvidenceChecks(base,token,(async()=>json({items:[]})) as typeof fetch),[]);
 for(const items of [[{...scope,canUpload:"true",canReview:false}],[{...scope,caseId:"../foreign",canUpload:true,canReview:false}]]){
  await assert.rejects(()=>listEvidenceChecks(base,token,(async()=>json({items})) as typeof fetch),EvidenceRequestError);
 }
});

test("invalid local file/scope or malformed server session is rejected before PUT",async()=>{
 for(const changed of [{...file,size:0},{...file,size:MAX_EVIDENCE_BYTES+1},{...file,mimeType:"text/html"}]){
  await assert.rejects(()=>uploadEvidence({apiBase:base,accessToken:token,scope,file:changed,transport:(async()=>{assert.fail("invalid file sent to server")}) as typeof fetch,put:async()=>{assert.fail("invalid file uploaded")}}),EvidenceRequestError);
 }
 for(const data of [{...initialized,caseId:"foreign-case"},{...initialized,uploadSessionId:"missing"},{...initialized,uploads:[{...initialized.uploads[0],headers:{}}]}]){
  await assert.rejects(()=>uploadEvidence({apiBase:base,accessToken:token,scope,file,transport:(async()=>json(data)) as typeof fetch,put:async()=>{assert.fail("malformed session uploaded")}}),EvidenceRequestError);
 }
});
