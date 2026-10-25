import copy
import unittest

from validate import validate_annotations


class AnnotationChecks(unittest.TestCase):
    def setUp(self):
        self.manifest={"videos":[{"id":"sample","duration_ms":20000,"scenes":[{"id":"s1","start_ms":0,"end_ms":20000}]}]}
        self.query={"id":"q1","query":"A red cup","category":"visual","scope":"corpus","answerable":True,"targets":[{"media_id":"sample","scene_id":"s1","start_ms":1000,"end_ms":20000,"acceptable_jump_ms":[0,2000]}]}

    def test_valid_half_open_end_and_negative(self):
        negative={"id":"q2","query":"A spacecraft","category":"negative","scope":"corpus","answerable":False,"targets":[]}
        validate_annotations(self.manifest,[self.query,negative])

    def test_invalid_targets_are_rejected(self):
        for update in ({"media_id":"missing"},{"scene_id":"missing"},{"start_ms":-1},{"start_ms":20000},{"end_ms":20001},{"start_ms":1.5},{"acceptable_jump_ms":[0,20001]}):
            with self.subTest(update=update):
                bad=copy.deepcopy(self.query)
                bad["targets"][0].update(update)
                with self.assertRaises(ValueError):
                    validate_annotations(self.manifest,[bad])

    def test_mislabelled_negative_and_duplicate_ids(self):
        bad=copy.deepcopy(self.query)
        bad["answerable"]=False
        with self.assertRaises(ValueError):
            validate_annotations(self.manifest,[bad])
        with self.assertRaises(ValueError):
            validate_annotations(self.manifest,[self.query,self.query])


if __name__=="__main__":
    unittest.main()
