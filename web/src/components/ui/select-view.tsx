import {Combobox} from '@base-ui/react/combobox';
import {Select} from '@base-ui/react/select';
import {t} from '../../i18n';
import {Icon} from '../../intelligent-ui/icons';
import {usePopupZoom} from './popup-zoom';
export type SelectOption={value:string;label:string;disabled:boolean;lang:string};
export type SelectViewProps={id:string;label:string;value:string;options:SelectOption[];disabled:boolean;busy:boolean;open:boolean;language:boolean;invalid?:boolean;container?:HTMLElement;variant?:'default'|'inline';setOpen:(value:boolean)=>void;choose:(value:string)=>void};

// Base UI owns search, positioning, focus, selection and popup dismissal.
export function SelectView({id,label,value,options,disabled,busy,open,language,invalid,container,variant='default',setOpen,choose}:SelectViewProps){
 const positionerStyle=usePopupZoom();
 // Short lists use the library's ordinary Select and native typeahead. Search
 // is a density policy, not another picker implementation or a user setting.
 if(options.filter(option=>option.value!=='').length<=12)return <Select.Root items={options} value={value} disabled={disabled} open={open} onOpenChange={setOpen} onValueChange={option=>{if(option!==null)choose(option)}}>
  <Select.Trigger id={id} className={'control-select-trigger'+(variant==='inline'?' control-select-inline':'')} aria-label={label} aria-invalid={invalid||undefined} aria-busy={busy} data-language-trigger={language?'':undefined} aria-disabled={disabled}>
   <span><Select.Value/></span><Select.Icon><Icon name="down"/></Select.Icon>
  </Select.Trigger>
  <Select.Portal container={container}><Select.Positioner className="control-select-positioner" style={positionerStyle} positionMethod="fixed" align="end" alignItemWithTrigger={false} sideOffset={5} collisionPadding={8}>
   <Select.Popup className="control-select-menu" data-language-popup={language?'':undefined}>
    <Select.List className="control-select-options" aria-label={t('Options')}>{options.map(option=><Select.Item key={option.value} value={option.value} disabled={option.disabled} lang={option.lang||undefined} data-language-option={language?option.value:undefined} className="control-select-option"><Select.ItemText>{option.label}</Select.ItemText><Select.ItemIndicator><Icon name="check"/></Select.ItemIndicator></Select.Item>)}</Select.List>
   </Select.Popup>
  </Select.Positioner></Select.Portal>
 </Select.Root>;
 return <Combobox.Root items={options} value={options.find(option=>option.value===value)??null} isItemEqualToValue={(a,b)=>a.value===b.value} disabled={disabled} open={open} onOpenChange={setOpen} onValueChange={option=>{if(option&&!option.disabled)choose(option.value)}}>
  <Combobox.Trigger id={id} className={'control-select-trigger'+(variant==='inline'?' control-select-inline':'')} aria-label={label} aria-invalid={invalid||undefined} aria-busy={busy} data-language-trigger={language?'':undefined} aria-disabled={disabled}>
   <span><Combobox.Value/></span><Combobox.Icon><Icon name="down"/></Combobox.Icon>
  </Combobox.Trigger>
  <Combobox.Portal container={container}><Combobox.Positioner className="control-select-positioner" style={positionerStyle} positionMethod="fixed" align="end" sideOffset={5} collisionPadding={8}>
   <Combobox.Popup className="control-select-menu" data-language-popup={language?'':undefined}>
    <Combobox.Input type="search" className="control-select-search" placeholder={t('Search options')} aria-label={t('Search options')} autoComplete="off"/>
    <Combobox.Empty className="control-select-empty" role="status">{t('No results.')}</Combobox.Empty>
    <Combobox.List id={language?'language-options':id+'-options'} className="control-select-options" aria-label={t('Options')}>
     {(option:SelectOption)=><Combobox.Item key={option.value} value={option} disabled={option.disabled} lang={option.lang||undefined} data-language-option={language?option.value:undefined} className="control-select-option"><span>{option.label}</span><Combobox.ItemIndicator><Icon name="check"/></Combobox.ItemIndicator></Combobox.Item>}
    </Combobox.List>
   </Combobox.Popup>
  </Combobox.Positioner></Combobox.Portal>
 </Combobox.Root>;
}
