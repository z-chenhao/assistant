import {useLayoutEffect,useRef} from 'react';
import {flushSync} from 'react-dom';
import {createRoot} from 'react-dom/client';
import {Button} from './components/ui/button';
import {Textarea} from './components/ui/textarea';

type Draft={text:string;files:FileList|null;focused:boolean;start:number;end:number};
function Composer({sourceID,draft}:{sourceID:string;draft:Draft}) {
  const text=useRef<HTMLTextAreaElement>(null),file=useRef<HTMLInputElement>(null);
  useLayoutEffect(()=>{
    if(file.current&&draft.files?.length)file.current.files=draft.files;
    if(text.current&&draft.focused){
      text.current.focus({preventScroll:true});
      text.current.setSelectionRange(draft.start,draft.end);
    }
  },[draft]);
  return <form className="composer" action="/message" method="post" encType="multipart/form-data">
    <input type="hidden" name="source_id" value={sourceID}/>
    <label htmlFor="message" className="sr-only">Message Erii</label>
    <Textarea ref={text} defaultValue={draft.text} id="message" name="text" placeholder="Ask anything, or hand something over…" required rows={2} maxLength={16384}/>
    <div className="composer-actions"><label className="attachment-picker" title="Attach a file"><span aria-hidden="true">＋</span><span className="sr-only">Attach a file</span><input ref={file} type="file" name="file" aria-label="Attach a file"/></label><span className="composer-hint">Your conversation, across every channel</span><Button type="submit" size="icon" className="send-button" aria-label="Send message">↑</Button></div>
  </form>;
}

export function enhanceComposer(){
 const composer=document.querySelector<HTMLElement>('#composer-root');
 if(!composer?.dataset.sourceId)return;
 const text=composer.querySelector<HTMLTextAreaElement>('textarea');
 const files=composer.querySelector<HTMLInputElement>('input[type="file"]')?.files??null;
 const draft={text:text?.value??'',files,focused:document.activeElement===text,start:text?.selectionStart??0,end:text?.selectionEnd??0};
 // Snapshot and commit in one task, preserving input entered before JS loaded.
 flushSync(()=>createRoot(composer).render(<Composer sourceID={composer.dataset.sourceId!} draft={draft}/>));
}
