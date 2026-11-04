import {enhanceWorkers} from './workers';
import {enhanceComputerView} from './computer';
import {enhanceShell} from './shell';
import {enhanceBrandMotion} from './brand';
import {enhanceSettings} from './settings';
import {enhanceCapabilityForms} from './capability-forms';
import {enhanceObservability} from './observability';
import {enhanceComposer} from './composer';
import {enhanceProjections} from './projections';
import {startLiveUpdates} from './live-updates';
import {enhanceRichForms} from './rich-forms';

// Capture server markup before enhancement, so unchanged React islands remain
// mounted. The composer has a separate lifetime from every live region.
enhanceShell();
enhanceComputerView();
enhanceWorkers();
const live=startLiveUpdates();
enhanceProjections();
enhanceRichForms();
enhanceComposer();
live?.start();

enhanceObservability();

enhanceSettings();
enhanceCapabilityForms();

enhanceBrandMotion();
