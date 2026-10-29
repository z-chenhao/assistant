import {buttonVariants,textareaStyle} from './components/ui/styles';
import {cn} from './lib/utils';

export function enhanceComposer(){
 const composer=document.querySelector<HTMLElement>('#composer-root');
 if(!composer?.dataset.sourceId)return;
 const text=composer.querySelector<HTMLTextAreaElement>('textarea');
 const submit=composer.querySelector<HTMLButtonElement>('button[type="submit"]');
 // Keep the form and every native input alive, including an open file picker.
 // No draft snapshot or React handoff can preserve an unfinished native selection.
 if(text)text.className=cn(textareaStyle,text.className);
 if(submit)submit.className=cn(buttonVariants({size:'icon'}),submit.className);
}
