# Hybrid local reader: (1) one full-context JSON-schema call (teacher prompt) for caecum/prep/polyps,
# (2) orchestrated histology question (snippets + negative examples + verbatim quote + diagnosis-lexicon check). Single pipeline.
import json, sys, time, urllib.request, os
from prompt import ONE, pj
import orch4b_lib as O
D=os.path.dirname(os.path.abspath(__file__)); port=sys.argv[1]; O.port=port
R=json.load(open(os.path.join(D,'..','data','evidence','reports43.json')))['reports']
E=lambda *v:{"type":"string","enum":list(v)}
SCHEMA={"type":"object","properties":{"caecum_reached":E("yes","no","not_stated"),"caecum_quote":{"type":"string","maxLength":160},
 "prep":E("adequate","inadequate","not_stated"),"prep_quote":{"type":"string","maxLength":160},"polyps_removed":E("yes","no"),"polyps_quote":{"type":"string","maxLength":160},
 "histology_present":E("yes","no"),"histology_quote":{"type":"string","maxLength":160}},
 "required":["caecum_reached","caecum_quote","prep","prep_quote","polyps_removed","polyps_quote","histology_present","histology_quote"]}
out=open(os.path.join(D,'out_gemma4b_hybrid.jsonl'),'w')
for r in R:
    t0=time.time()
    b={"messages":[{"role":"user","content":ONE.format(t=r['text'])}],"temperature":0,"seed":1,"max_tokens":400,
       "response_format":{"type":"json_schema","json_schema":{"name":"ev","schema":SCHEMA}}}
    rq=urllib.request.Request(f'http://127.0.0.1:{port}/v1/chat/completions',json.dumps(b).encode(),{'Content-Type':'application/json'})
    j=pj(json.load(urllib.request.urlopen(rq,timeout=600))['choices'][0]['message']['content'])
    f=dict(caecum=j.get('caecum_reached'),prep=j.get('prep'),polyps=j.get('polyps_removed'))
    a,qt=O.ask(r['text'],'histology',0); f['histology']=O.validate(r['text'],'histology',a,qt)
    st=O.state(f['caecum'],f['prep'],f['polyps'],f['histology']); dt=time.time()-t0
    rec=dict(uid=r['uid'],sha=r['sha'],sec=round(dt,3),parsed=True,fields=f,state=st[0],reason=st[1],
             gemma=dict(state=r['gemma']['state'],reason=r['gemma']['reason'],**{k:r['gemma'][k] for k in ('caecum','prep','polyps','histology')}),
             regex=dict(state=r['regex']['state'],reason=r['regex']['reason']),prompt_tokens=0,completion_tokens=0,prompt_tps=0,gen_tps=0,raw=[j,a,qt])
    out.write(json.dumps(rec)+'\n'); out.flush(); print(r['uid'],round(dt,1),st,flush=True)
