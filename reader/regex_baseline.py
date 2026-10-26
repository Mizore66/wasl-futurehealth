# Frozen regex baseline (written before any model output was seen). Fields as in labels_human.json.
import re
SEG=r'(?:distal|proximal|mid)?\s*(?:descending|sigmoid|transverse|splenic|hepatic|ascending|rectosigmoid)'
def caecum(t):
    if re.search(r'(?:unable to|could not|failed to|not able to) (?:reach|advance|intubate|pass)|aborted|incomplete (?:exam|colonoscopy|study)|(?:passed|advanced|inserted)[^.]{0,40}to the '+SEG,t,re.I): return 'no'
    if re.search(r'c(?:a)?ec(?:um|al)|ileoc(?:a)?ecal|terminal ileum|appendiceal orifice',t,re.I) and re.search(r'colonoscop|scope',t,re.I): return 'yes'
    return 'not_stated'
def prep(t):
    if re.search(r'(?:poor|inadequate|suboptimal|unsatisfactory)[^.]{0,30}prep|prep[a-z]*[^.]{0,30}(?:poor|inadequate|suboptimal|unsatisfactory)',t,re.I): return 'inadequate'
    if re.search(r'(?:good|excellent|adequate|fair|satisfactory)[^.]{0,30}prep|prep[a-z]*[^.]{0,30}(?:good|excellent|adequate|fair|satisfactory)|well[- ]prep|boston bowel prep[^.]{0,20}\d',t,re.I): return 'adequate'
    return 'not_stated'
def polyps(t):
    if re.search(r'polypectomy|polyps?[^.]{0,120}(?:removed|resected|excised|snared|fulgurated|ablated|retrieved)|(?:removed|resected|excised|snared)[^.]{0,60}polyp',t,re.I): return 'yes'
    return 'no'
def histology(t):
    if re.search(r'tubular|villous|adenocarcinoma|hyperplastic polyp|sessile serrated|dysplasia|carcinoma|microscopic (?:description|examination)|histolog[a-z]* (?:shows|showed|revealed|confirm)',t,re.I): return 'yes'
    return 'no'
def predict(t): return {'caecum':caecum(t),'prep':prep(t),'polyps':polyps(t),'histology':histology(t)}
