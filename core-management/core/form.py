from django import forms

MAX_VIDEO_CHUNK_SIZE = 1 * 1024 * 1024

class VideoChunkUploadForm(forms.Form):
    chunk = forms.FileField(required=True)
    chunkIndex = forms.IntegerField(min_value=0, required=True)

    def clean_chunk(self):
        chunk = self.cleaned_data.get('chunk')
        
        if chunk.size > MAX_VIDEO_CHUNK_SIZE:
            raise forms.ValidationError('O arquivo deve ser um vídeo no formato MP4.')

        return chunk
    
class VideoChunkFinishUploadForm(forms.Form):
    fileName = forms.CharField(max_length=255, required=True)
    totalChunks = forms.IntegerField(min_value=1, required=True)