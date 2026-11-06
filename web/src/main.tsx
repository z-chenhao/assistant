import {enhanceFilterForms} from './components/ui/filter-form';
import {enhanceTimeDetails} from './components/ui/time-detail';
import {enhanceLocalTimes} from './local-time';
import {enhanceContextHelp} from './components/ui/help';
import {enhanceWorkers} from './workers';
import {enhanceAppConnections} from './app-connections';
import {enhanceComputerView} from './computer';
import {enhanceShell} from './shell';
import {enhanceBrandMotion} from './brand';
import {enhanceSettings} from './settings';
import {enhanceFormDisclosures} from './components/ui/disclosure';
import {enhanceSelectControls} from './components/ui/select';
import {enhanceCapabilityForms} from './capability-forms';
import {enhanceObservability} from './observability';
import {enhanceMemory} from './memory';
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
enhanceLocalTimes();
enhanceTimeDetails();
enhanceProjections();
enhanceRichForms();
enhanceComposer(live?.messageAccepted);
live?.start();

enhanceObservability();
enhanceMemory();

enhanceAppConnections();
enhanceContextHelp();
enhanceSettings();
enhanceFormDisclosures();
enhanceSelectControls();
enhanceFilterForms();
enhanceCapabilityForms();

enhanceBrandMotion();
