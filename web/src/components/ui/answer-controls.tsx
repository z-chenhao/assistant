// Adapted from shadcn/ui @ 8a3536678a528cc012b9e62484290e79d0be410f.
// MIT; see web/licenses/shadcn-ui.txt. Native form semantics and Steward tokens.
import type {ComponentProps} from 'react';
import {cn} from '../../lib/utils';
import {Button} from './button';

export function Field({className,...props}:ComponentProps<'div'>){
 return <div role="group" data-slot="field" className={cn('native-form-field',className)} {...props}/>;
}
export function FieldLabel(props:ComponentProps<'label'>){
 return <label data-slot="field-label" {...props}/>;
}
export function Input({className,type,...props}:ComponentProps<'input'>){
 return <input type={type} data-slot="input" className={cn('answer-input',className)} {...props}/>;
}
export function Textarea({className,...props}:ComponentProps<'textarea'>){
 return <textarea data-slot="textarea" className={cn('answer-input',className)} {...props}/>;
}
export function AnswerButton({className,variant='default',...props}:ComponentProps<'button'>&{variant?:'default'|'secondary'}){
 return <Button data-slot="button" data-variant={variant} variant={variant} className={cn('answer-button',className)} {...props}/>;
}
export function ResultCard({className,...props}:ComponentProps<'article'>){
 return <article data-slot="card" className={cn('native-result-card',className)} {...props}/>;
}
