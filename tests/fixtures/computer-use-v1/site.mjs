import {createServer} from 'node:http';
createServer((request,response)=>{
 if(request.url==='/download'){response.writeHead(200,{'Content-Type':'text/plain','Content-Disposition':'attachment; filename=fixture.txt'});response.end('Guest browser download fixture');return;}
 response.setHeader('Content-Type','text/html');
 response.end('<title>Collie Guest Browser Fixture</title><input id="field" aria-label="Fixture input"><button id="button" onclick="document.querySelector(\'#status\').textContent=\'clicked\'">Click</button><div id="status">ready</div><a href="/download" id="download">Download</a><button id="upload" onclick="document.querySelector(\'#file\').click()">Upload</button><input id="file" type="file" onchange="document.querySelector(\'#status\').textContent=this.files[0].name">');
}).listen(18025,'127.0.0.1');
