import {Menu} from '@base-ui/react/menu';
import {t} from '../../i18n';
import {useLayoutEffect,useRef,useState,type ReactNode} from 'react';
import {Icon} from '../../intelligent-ui/icons';
import {usePopupZoom} from './popup-zoom';
export type MenuAction={label:string;run:()=>void|Promise<void>;icon?:ReactNode;disabled?:boolean};
export function ActionMenu({actions,label='Options'}:{actions:MenuAction[];label?:string}){
 const trigger=useRef<HTMLButtonElement>(null),[container,setContainer]=useState<HTMLElement>();
 const positionerStyle=usePopupZoom();
 useLayoutEffect(()=>setContainer(trigger.current?.closest('dialog')??undefined),[]);
 return <Menu.Root><Menu.Trigger ref={trigger} className="answer-icon-button" aria-label={t(label)} title={t(label)}><Icon name="more"/></Menu.Trigger>
  <Menu.Portal container={container}><Menu.Positioner className="control-select-positioner" style={positionerStyle} positionMethod="fixed" align="end" sideOffset={5} collisionPadding={8}>
   <Menu.Popup className="shared-action-menu">{actions.map((action,index)=><Menu.Item key={index} disabled={action.disabled} onClick={()=>void action.run()}>{action.icon}{t(action.label)}</Menu.Item>)}</Menu.Popup>
  </Menu.Positioner></Menu.Portal>
 </Menu.Root>;
}
