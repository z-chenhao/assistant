const assets = ASSETS;
const inputs = [];
for (const asset of assets) {
 const saved = await tools.builtin__artifact__manage({...asset,operation:"save"});
 inputs.push(...saved.artifactRefs);
}
const rows = assets[0].text.trimEnd().split("\n").slice(1).map(line => {
 const [row_id,text,attachment_ref] = line.split(","); return {row_id,text,attachment_ref};
});
async function classify(row, hold) {
 const result = await agent.run({task:"Classify feedback: " + JSON.stringify({...row, hold}), artifactRefs:inputs});
 return JSON.parse(result.text);
}
const first = await Promise.all(rows.slice(0,37).map(row=>classify(row,false)));
const active = await Promise.all(rows.slice(37,45).map(row=>classify(row,true)));
const rest = await Promise.all(rows.slice(45).map(row=>classify(row,false)));
const results = [...first,...active,...rest].map(row=>({...row,evidence:row.row_id}));
const columns = ["row_id","category","severity","evidence","attachment_ref"];
function quote(x) { return '"' + String(x).replace(/"/g,'""') + '"'; }
const csv = columns.join(",")+"\n"+results.map(row=>columns.map(c=>quote(row[c])).join(",")).join("\n")+"\n";
const saved = await tools.builtin__artifact__manage({operation:"save",name:"feedback_analysis.csv",mediaType:"text/csv",text:csv});
await tools.builtin__artifact__manage({operation:"publish",artifactRefs:saved.artifactRefs});
const categories = {}, severities = {};
for (const row of results) {
 categories[row.category] = (categories[row.category] || 0) + 1;
 severities[row.severity] = (severities[row.severity] || 0) + 1;
}
return {count:results.length,categories,severities,artifactRefs:saved.artifactRefs};
