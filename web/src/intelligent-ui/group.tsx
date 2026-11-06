import {createContext,useContext,type ReactNode} from 'react';
import {useUIState} from './state';
export type ToolAction={reference:string;id:string;label:string;tool:string;arguments:Record<string,unknown>};
type Group={query:string;selected:string;setQuery:(q:string)=>void;select:(label:string)=>void;actions:ToolAction[]};
const context=createContext<Group>({query:'',selected:'',setQuery:()=>{},select:()=>{},actions:[]});
export function InteractionGroup({reference,actions,children}:{reference:string;actions:ToolAction[];children:ReactNode}){
 const [state,update]=useUIState(reference,{query:'',selected:''},s=>({filter:s.query,selection:s.selected}));
 return <context.Provider value={{query:state.query,selected:state.selected,setQuery:query=>update({query}),select:selected=>update({selected:state.selected===selected?'':selected}),actions}}>{children}</context.Provider>
}
export const useInteractionGroup=()=>useContext(context);
